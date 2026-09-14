package lostresource

import (
	"go/ast"
	"go/types"
	"slices"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/ctrlflow"
	"golang.org/x/tools/go/types/typeutil"
)

// resultFact records guarantees proved at every reachable return of a function.
// Entries follow result positions; no syntax or type-checker pointers cross passes.
type resultFact struct {
	Results []resultGuarantee
}

// resultGuarantee describes when a result cannot contain an acquired resource.
type resultGuarantee struct {
	AlwaysNil  bool
	NilOnError bool
	NilOnFalse bool
}

// AFact identifies serializable function facts to the analysis driver.
func (*resultFact) AFact() {}

// resultGuarantees proves simple return contracts without guessing from names.
// Interface dispatch and named returns affected by defers remain unknown.
func resultGuarantees(pass *analysis.Pass, resources []Resource) {
	for exportResultGuarantees(pass, resources) {
	}
}

// exportResultGuarantees grows the local summaries to a fixed point so declaration
// order does not affect forwarding wrappers. Unknown recursive cycles stay unknown.
func exportResultGuarantees(pass *analysis.Pass, resources []Resource) bool {
	changed := false
	graphs := pass.ResultOf[ctrlflow.Analyzer].(*ctrlflow.CFGs)
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			obj := pass.TypesInfo.ObjectOf(fn.Name).(*types.Func)
			results := obj.Type().(*types.Signature).Results()
			matched := false
			for v := range results.Variables() {
				for _, resource := range resources {
					matched = matched || typeName(v.Type()) == resource.Type
				}
			}
			if !matched {
				continue
			}
			named := false
			for v := range results.Variables() {
				named = named || v.Name() != ""
			}
			hasDefer := false
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if _, ok := n.(*ast.FuncLit); ok {
					return false
				}
				_, deferred := n.(*ast.DeferStmt)
				hasDefer = hasDefer || deferred
				return true
			})
			if named && hasDefer {
				continue
			}

			fact := resultFact{Results: make([]resultGuarantee, results.Len())}
			for i := range fact.Results {
				fact.Results[i] = resultGuarantee{true, true, true}
			}
			returns := 0
			for _, block := range graphs.FuncDecl(fn).Blocks {
				if !block.Live {
					continue
				}
				ret := block.Return()
				if ret == nil {
					continue
				}
				returns++
				for i := range fact.Results {
					known := resultGuarantee{}
					if len(ret.Results) == 1 {
						if call, ok := ast.Unparen(ret.Results[0]).(*ast.CallExpr); ok {
							known = callGuarantee(pass, call, i)
						}
					}
					if len(ret.Results) == results.Len() && !known.AlwaysNil && !known.NilOnError && !known.NilOnFalse {
						known.AlwaysNil = pass.TypesInfo.Types[ret.Results[i]].IsNil()
						last := ret.Results[len(ret.Results)-1]
						known.NilOnError = known.AlwaysNil || pass.TypesInfo.Types[last].IsNil()
						if len(ret.Results) > 1 {
							value := pass.TypesInfo.Types[ret.Results[len(ret.Results)-2]].Value
							known.NilOnFalse = known.AlwaysNil || value != nil && value.ExactString() == "true"
						}
					}
					fact.Results[i].AlwaysNil = fact.Results[i].AlwaysNil && known.AlwaysNil
					fact.Results[i].NilOnError = fact.Results[i].NilOnError && known.NilOnError
					fact.Results[i].NilOnFalse = fact.Results[i].NilOnFalse && known.NilOnFalse
				}
			}
			useful := false
			for _, result := range fact.Results {
				useful = useful || result.AlwaysNil || result.NilOnError || result.NilOnFalse
			}
			if returns != 0 && useful {
				var previous resultFact
				if !pass.ImportObjectFact(obj, &previous) || !slices.Equal(previous.Results, fact.Results) {
					pass.ExportObjectFact(obj, &fact)
					changed = true
				}
			}
		}
	}
	return changed
}

// callGuarantee reads a statically resolved callee's result contract.
func callGuarantee(pass *analysis.Pass, call *ast.CallExpr, index int) resultGuarantee {
	fn := typeutil.StaticCallee(pass.TypesInfo, call)
	var fact resultFact
	if fn != nil && pass.ImportObjectFact(fn.Origin(), &fact) && index < len(fact.Results) {
		return fact.Results[index]
	}
	return resultGuarantee{}
}
