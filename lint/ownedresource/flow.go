package ownedresource

import (
	"go/ast"
	"go/token"
	"go/types"
	"maps"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/ctrlflow"
	"golang.org/x/tools/go/cfg"
)

// Flow searches for a path that loses one acquired resource. Each search tracks
// local aliases independently, so an overwrite cannot release an older value.
type Flow struct {
	// pass supplies resolved identifiers and diagnostics.
	pass *analysis.Pass
	// graphs supplies the CFGs, including nested literal functions.
	graphs *ctrlflow.CFGs
	// resource defines cleanup and transfer for the acquired result.
	resource Resource
	// scope bounds local variables; assignments outside it transfer ownership.
	scope *types.Scope
	// results identifies named returns that transfer ownership on a bare return.
	results *types.Tuple
}

// flowState contains the aliases and delayed closures on a single CFG path.
type flowState struct {
	// aliases identifies variables currently holding this acquisition.
	aliases map[*types.Var]bool
	// deferred contains cleanup closures whose free variables are read on return.
	deferred []*ast.FuncLit
	// err identifies the unchanged acquisition error for NilOnError contracts.
	err *types.Var
}

// NewFlow constructs a path checker for a validated Resource contract and an
// analysis pass requiring ctrlflow.
func NewFlow(pass *analysis.Pass, resource Resource) *Flow {
	return &Flow{pass: pass, graphs: pass.ResultOf[ctrlflow.Analyzer].(*ctrlflow.CFGs), resource: resource}
}

// Check reports a witness path that loses v, acquired by call in def.
// The definition must be an assignment or value specification from this pass.
func (f *Flow) Check(def ast.Node, call *ast.CallExpr, v *types.Var) {
	// Select the innermost function, preserving package and closure transfers.
	var graph *cfg.CFG
	for _, file := range f.pass.Files {
		if def.Pos() < file.Pos() || def.End() > file.End() {
			continue
		}
		ast.Inspect(file, func(n ast.Node) bool {
			if n == nil || def.Pos() < n.Pos() || def.End() > n.End() {
				return false
			}
			var ft *ast.FuncType
			var sig *types.Signature
			switch n := n.(type) {
			case *ast.FuncDecl:
				ft = n.Type
				graph = f.graphs.FuncDecl(n)
				sig, _ = f.pass.TypesInfo.ObjectOf(n.Name).Type().(*types.Signature)
			case *ast.FuncLit:
				ft = n.Type
				graph = f.graphs.FuncLit(n)
				sig, _ = f.pass.TypesInfo.TypeOf(n).(*types.Signature)
			}
			if ft != nil {
				f.scope = f.pass.TypesInfo.Scopes[ft]
				f.results = sig.Results()
			}
			return true
		})
	}
	if graph == nil || f.scope == nil || !f.scope.Contains(v.Pos()) {
		return
	}

	// Start immediately after this acquisition, ignoring unreachable definitions.
	state := flowState{aliases: map[*types.Var]bool{v: true}}
	lhs, _ := bindings(def)
	results, _ := f.pass.TypesInfo.TypeOf(call).(*types.Tuple)
	if f.resource.NilOnError && results != nil && results.Len() > 1 && len(lhs) == results.Len() && types.Identical(results.At(results.Len()-1).Type(), types.Universe.Lookup("error").Type()) {
		if id, ok := lhs[len(lhs)-1].(*ast.Ident); ok {
			state.err, _ = f.pass.TypesInfo.ObjectOf(id).(*types.Var)
		}
	}
	for _, block := range graph.Blocks {
		if !block.Live {
			continue
		}
		for i, node := range block.Nodes {
			if node != def {
				continue
			}
			if loss := f.search(block, i+1, state, make(map[string]bool)); loss != nil {
				f.pass.Report(analysis.Diagnostic{
					Pos: call.Pos(), End: call.End(),
					Message: "owned " + v.Name() + " from " + functionName(f.pass.TypesInfo, call) + " is not released or transferred on all paths; use " + f.resource.cleanup(),
					Related: []analysis.RelatedInformation{{Pos: loss.Pos(), Message: "this path loses " + v.Name() + " without releasing or transferring it"}},
				})
			}
			return
		}
	}
}

// search follows live CFG edges until release, transfer, overwrite, or return.
// Its memo includes aliases and delayed closures, since those can differ at a join.
func (f *Flow) search(block *cfg.Block, start int, state flowState, seen map[string]bool) ast.Node {
	// Memoize the complete path state to terminate loops without merging aliases.
	positions := make([]int, 0, len(state.aliases))
	for v := range state.aliases {
		positions = append(positions, int(v.Pos()))
	}
	slices.Sort(positions)
	var key strings.Builder
	key.WriteString(strconv.Itoa(int(block.Index)) + ":" + strconv.Itoa(start))
	for _, pos := range positions {
		key.WriteString("," + strconv.Itoa(pos))
	}
	for _, lit := range state.deferred {
		key.WriteString(";" + strconv.Itoa(int(lit.Pos())))
	}
	if state.err != nil {
		key.WriteString("e" + strconv.Itoa(int(state.err.Pos())))
	}
	if seen[key.String()] {
		return nil
	}
	seen[key.String()] = true
	state.aliases = maps.Clone(state.aliases)
	state.deferred = slices.Clone(state.deferred)

	// Evaluate each operation before its writes, matching parallel assignment.
	for _, node := range block.Nodes[start:] {
		if f.effects(node, &state) {
			return nil
		}
		if ret, ok := node.(*ast.ReturnStmt); ok {
			if ret.Results == nil && f.results != nil {
				for v := range f.results.Variables() {
					if state.aliases[v] {
						return nil
					}
				}
			}
			for _, result := range ret.Results {
				if f.transfers(result, state) {
					return nil
				}
			}
			for _, lit := range slices.Backward(state.deferred) {
				if f.closureReleases(lit, state) {
					return nil
				}
				f.forgetWrites(lit, state)
			}
			return ret
		}
		if send, ok := node.(*ast.SendStmt); ok && f.transfers(send.Value, state) {
			return nil
		}
		lhs, rhs := bindings(node)
		if len(lhs) == 0 {
			continue
		}
		next := maps.Clone(state.aliases)
		for i, target := range lhs {
			var value ast.Expr
			if len(lhs) == len(rhs) {
				value = rhs[i]
			}
			id, local := target.(*ast.Ident)
			if local && id.Name == "_" {
				continue
			}
			var v *types.Var
			if local {
				v, _ = f.pass.TypesInfo.ObjectOf(id).(*types.Var)
			}
			if v != nil && v == state.err {
				state.err = nil
			}
			if value != nil && f.transfers(value, state) {
				if v == nil || !f.scope.Contains(v.Pos()) || !f.alias(value, state) {
					return nil
				}
				next[v] = true
				continue
			}
			delete(next, v)
		}
		state.aliases = next
		if len(next) == 0 {
			return node
		}
	}

	// A live handle cannot take a nil branch; other error correlations require
	// an explicit NilOnError contract. General boolean relations stay conservative.
	for i, succ := range block.Succs {
		if len(block.Succs) == 2 && len(block.Nodes) != 0 {
			if value, known := f.condition(block.Nodes[len(block.Nodes)-1], state); known && value != (i == 0) {
				continue
			}
		}
		if loss := f.search(succ, 0, state, seen); loss != nil {
			return loss
		}
	}
	return nil
}

// forgetWrites drops aliases that an earlier-running defer may replace before
// another cleanup closure reads them. Conditional writes are conservative.
func (f *Flow) forgetWrites(lit *ast.FuncLit, state flowState) {
	ast.Inspect(lit.Body, func(node ast.Node) bool {
		if _, ok := node.(*ast.FuncLit); ok {
			return false
		}
		lhs, rhs := bindings(node)
		for i, target := range lhs {
			id, ok := target.(*ast.Ident)
			if !ok {
				continue
			}
			v, _ := f.pass.TypesInfo.ObjectOf(id).(*types.Var)
			if state.aliases[v] && (len(lhs) != len(rhs) || !f.alias(rhs[i], state)) {
				delete(state.aliases, v)
			}
		}
		return true
	})
}

// alias recognizes a local handle through parentheses and type conversions.
func (f *Flow) alias(expr ast.Expr, state flowState) bool {
	switch expr := ast.Unparen(expr).(type) {
	case *ast.Ident:
		v, _ := f.pass.TypesInfo.ObjectOf(expr).(*types.Var)
		return state.aliases[v]
	case *ast.CallExpr:
		return f.pass.TypesInfo.Types[expr.Fun].IsType() && len(expr.Args) == 1 && f.alias(expr.Args[0], state)
	}
	return false
}

// transfers recognizes a handle returned or stored inside an aggregate.
// Aggregate lifetimes are the receiving component's concern.
func (f *Flow) transfers(expr ast.Expr, state flowState) bool {
	if f.alias(expr, state) {
		return true
	}
	switch expr := ast.Unparen(expr).(type) {
	case *ast.UnaryExpr:
		return expr.Op == token.AND && f.transfers(expr.X, state)
	case *ast.CompositeLit:
		for _, elt := range expr.Elts {
			if kv, ok := elt.(*ast.KeyValueExpr); ok {
				elt = kv.Value
			}
			if f.transfers(elt, state) {
				return true
			}
		}
	}
	return false
}

// effects recognizes calls to explicit cleanup and transfer contracts.
func (f *Flow) effects(node ast.Node, state *flowState) bool {
	// A deferred literal reads captured variables later; a method defer captures
	// its receiver now. Keep those lifetimes distinct when a variable is overwritten.
	if stmt, ok := node.(*ast.DeferStmt); ok {
		if lit, ok := ast.Unparen(stmt.Call.Fun).(*ast.FuncLit); ok && len(stmt.Call.Args) == 0 {
			if !slices.Contains(state.deferred, lit) {
				state.deferred = append(state.deferred, lit)
			}
			return false
		}
	}

	// Inspect only executing expressions; uncalled literals cannot release a handle.
	done := false
	ast.Inspect(node, func(n ast.Node) bool {
		if done {
			return false
		}
		switch n := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.BinaryExpr:
			if n.Op == token.LAND || n.Op == token.LOR {
				// The CFG does not model short circuit edges. Credit only effects
				// in the operand guaranteed to execute on this path.
				done = f.effects(n.X, state)
				return false
			}
		case *ast.CallExpr:
			if lit, ok := ast.Unparen(n.Fun).(*ast.FuncLit); ok && len(n.Args) == 0 {
				done = f.closureReleases(lit, *state)
				return !done
			}
			name := functionName(f.pass.TypesInfo, n)
			for _, consumer := range f.resource.Consumers {
				fn, arg, _ := strings.Cut(consumer, ":")
				index, _ := strconv.Atoi(arg)
				if sel, ok := ast.Unparen(n.Fun).(*ast.SelectorExpr); ok {
					if selection := f.pass.TypesInfo.Selections[sel]; selection != nil && selection.Kind() == types.MethodExpr {
						index++
					}
				}
				if name == fn && index < len(n.Args) && f.transfers(n.Args[index], *state) {
					done = true
					return false
				}
			}
			if sel, ok := ast.Unparen(n.Fun).(*ast.SelectorExpr); ok {
				selection := f.pass.TypesInfo.Selections[sel]
				if selection != nil && slices.Contains(f.resource.ReleaseMethods, sel.Sel.Name) {
					done = selection.Kind() == types.MethodVal && f.alias(sel.X, *state)
					if selection.Kind() == types.MethodExpr && len(n.Args) != 0 {
						done = f.alias(n.Args[0], *state)
					}
				}
			}
			if (name == "testing.common.Cleanup" || name == "testing.TB.Cleanup") && len(n.Args) == 1 {
				if lit, ok := ast.Unparen(n.Args[0]).(*ast.FuncLit); ok {
					if !slices.Contains(state.deferred, lit) {
						state.deferred = append(state.deferred, lit)
					}
				}
				if sel, ok := ast.Unparen(n.Args[0]).(*ast.SelectorExpr); ok && slices.Contains(f.resource.ReleaseMethods, sel.Sel.Name) {
					done = f.alias(sel.X, *state)
				}
			}
		}
		return !done
	})
	return done
}

// closureReleases checks all returning paths of an invoked cleanup literal.
func (f *Flow) closureReleases(lit *ast.FuncLit, state flowState) bool {
	graph := f.graphs.FuncLit(lit)
	inner := *f
	inner.results = nil
	state.deferred = nil
	return !graph.NoReturn() && inner.search(graph.Blocks[0], 0, state, make(map[string]bool)) == nil
}

// condition prunes only comparisons implied by a live acquisition.
func (f *Flow) condition(node ast.Node, state flowState) (value, known bool) {
	expr, ok := node.(ast.Expr)
	if !ok {
		return false, false
	}
	switch expr := ast.Unparen(expr).(type) {
	case *ast.UnaryExpr:
		if expr.Op == token.NOT {
			value, known = f.condition(expr.X, state)
			return !value, known
		}
	case *ast.BinaryExpr:
		if expr.Op != token.EQL && expr.Op != token.NEQ {
			return false, false
		}
		x, y := expr.X, expr.Y
		if f.pass.TypesInfo.Types[x].IsNil() {
			x, y = y, x
		}
		if !f.pass.TypesInfo.Types[y].IsNil() {
			return false, false
		}
		if f.alias(x, state) {
			return expr.Op == token.NEQ, true
		}
		if id, ok := ast.Unparen(x).(*ast.Ident); ok && state.err != nil && f.pass.TypesInfo.ObjectOf(id) == state.err {
			return expr.Op == token.EQL, true
		}
	}
	return false, false
}
