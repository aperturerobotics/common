// Package protobufnil provides a golangci-lint module plugin that forbids
// comparing protobuf-go-lite message pointers against nil.
//
// In protobuf3, nil and a zero-valued message are indistinguishable on the
// wire, so a nil check on a message pointer cannot determine whether the
// field is set. The only permitted nil comparison is the initialization
// pattern that replaces a nil pointer with an empty message:
//
//	if msg.GetSub() == nil {
//		msg.Sub = &Sub{}
//	}
package protobufnil

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

func init() {
	register.Plugin("protobufnil", New)
}

type plugin struct{}

// New constructs the plugin.
func New(_ any) (register.LinterPlugin, error) {
	return &plugin{}, nil
}

// BuildAnalyzers returns the analysis passes.
func (p *plugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{{
		Name: "protobufnil",
		Doc:  "forbids nil comparisons on protobuf-go-lite message pointers (nil and unset are indistinguishable in protobuf3)",
		Run:  run,
	}}, nil
}

// GetLoadMode returns the load mode (type info is required).
func (p *plugin) GetLoadMode() string {
	return register.LoadModeTypesInfo
}

// protoMessagePointer reports whether the expression type is a pointer to a
// struct declared in a generated .pb.go file that implements
// protobuf_go_lite.Message.
func protoMessagePointer(pass *analysis.Pass, expr ast.Expr) bool {
	tv, ok := pass.TypesInfo.Types[expr]
	if !ok {
		return false
	}
	ptr, ok := tv.Type.(*types.Pointer)
	if !ok {
		return false
	}
	named, ok := types.Unalias(ptr.Elem()).(*types.Named)
	if !ok {
		return false
	}
	if !types.Implements(ptr, msgIface) {
		return false
	}
	pos := named.Obj().Pos()
	if !pos.IsValid() {
		return false
	}
	return strings.HasSuffix(pass.Fset.Position(pos).Filename, ".pb.go")
}

// msgIface is the protobuf_go_lite.Message interface type, described
// structurally so the plugin does not need the runtime package as a type
// identity anchor.
var msgIface = func() *types.Interface {
	pkg := types.NewPackage("github.com/aperturerobotics/protobuf-go-lite", "protobuf_go_lite")
	sig := func(params, results *types.Tuple) *types.Signature {
		return types.NewSignatureType(nil, nil, nil, params, results, false)
	}
	v := func(name string, t types.Type) *types.Var {
		return types.NewVar(token.NoPos, pkg, name, t)
	}
	m := func(name string, sig *types.Signature) *types.Func {
		return types.NewFunc(token.NoPos, pkg, name, sig)
	}
	bytes := types.NewSlice(types.Typ[types.Byte])
	errType := types.Universe.Lookup("error").Type()
	methods := []*types.Func{
		m("SizeVT", sig(nil, types.NewTuple(v("", types.Typ[types.Int])))),
		m("MarshalVT", sig(nil, types.NewTuple(v("", bytes), v("", errType)))),
		m("UnmarshalVT", sig(
			types.NewTuple(v("data", bytes)),
			types.NewTuple(v("", errType)),
		)),
		m("Reset", sig(nil, nil)),
	}
	iface := types.NewInterfaceType(methods, nil)
	iface.Complete()
	return iface
}()

// nilOperand returns the non-nil operand of a comparison against nil, if any.
// receiverIdent reports whether the identifier is the receiver of the
// enclosing method. A nil receiver check guards against a nil call target and
// does not test whether a field is set, so it is permitted.
func receiverIdent(pass *analysis.Pass, expr ast.Expr) bool {
	id, ok := expr.(*ast.Ident)
	if !ok {
		return false
	}
	pos := id.Pos()
	for _, file := range pass.Files {
		if pos < file.Pos() || pos > file.End() {
			continue
		}
		found := false
		ast.Inspect(file, func(n ast.Node) bool {
			decl, ok := n.(*ast.FuncDecl)
			if !ok || decl.Recv == nil || pos < decl.Pos() || pos > decl.End() {
				return true
			}
			for _, field := range decl.Recv.List {
				for _, name := range field.Names {
					if name.Name == id.Name {
						found = true
						return false
					}
				}
			}
			return true
		})
		return found
	}
	return false
}
func nilOperand(bin *ast.BinaryExpr) ast.Expr {
	if bin.Op != token.EQL && bin.Op != token.NEQ {
		return nil
	}
	if id, ok := bin.X.(*ast.Ident); ok && id.Name == "nil" {
		return bin.Y
	}
	if id, ok := bin.Y.(*ast.Ident); ok && id.Name == "nil" {
		return bin.X
	}
	return nil
}

func run(pass *analysis.Pass) (any, error) {
	// Map comparison conditions to their if-statements so the initialization
	// exception can be applied.
	ifConds := map[*ast.BinaryExpr]*ast.IfStmt{}
	for _, file := range pass.Files {
		ast.Inspect(file, func(n ast.Node) bool {
			ifStmt, ok := n.(*ast.IfStmt)
			if !ok {
				return true
			}
			if bin, ok := ifStmt.Cond.(*ast.BinaryExpr); ok {
				ifConds[bin] = ifStmt
			}
			return true
		})
	}

	for _, file := range pass.Files {
		fname := pass.Fset.Position(file.Pos()).Filename
		if strings.HasSuffix(fname, ".pb.go") {
			// Generated code manages its own nil checks.
			continue
		}
		ast.Inspect(file, func(n ast.Node) bool {
			bin, ok := n.(*ast.BinaryExpr)
			if !ok {
				return true
			}
			operand := nilOperand(bin)
			if operand == nil || receiverIdent(pass, operand) || !protoMessagePointer(pass, operand) {
				return true
			}
			if ifStmt, ok := ifConds[bin]; ok && initPattern(pass, ifStmt, operand) {
				return true
			}
			pass.Report(analysis.Diagnostic{
				Pos: bin.Pos(),
				Message: "comparing a protobuf message pointer with nil: in protobuf3 nil and an unset " +
					"message are indistinguishable, so this check cannot determine presence; " +
					"use a presence flag or the initialization pattern (if x == nil { x = &T{} })",
			})
			return true
		})
	}
	return nil, nil
}

// initPattern reports whether the if-statement body assigns the compared
// expression to an empty message literal or new(T).
func initPattern(pass *analysis.Pass, ifStmt *ast.IfStmt, operand ast.Expr) bool {
	for _, stmt := range ifStmt.Body.List {
		assign, ok := stmt.(*ast.AssignStmt)
		if !ok {
			continue
		}
		for i, lhs := range assign.Lhs {
			if i >= len(assign.Rhs) || !sameType(pass, lhs, operand) {
				continue
			}
			if emptyMessageValue(pass, assign.Rhs[i], operand) {
				return true
			}
		}
	}
	return false
}

// emptyMessageValue reports whether the expression is &T{} or new(T) where T
// matches the type of the compared operand.
func emptyMessageValue(pass *analysis.Pass, expr, operand ast.Expr) bool {
	tv, ok := pass.TypesInfo.Types[expr]
	if !ok {
		return false
	}
	to, ok := pass.TypesInfo.Types[operand]
	if !ok {
		return false
	}
	return types.Identical(tv.Type, to.Type)
}

// sameType reports whether both expressions have the same static type.
func sameType(pass *analysis.Pass, a, b ast.Expr) bool {
	ta, ok := pass.TypesInfo.Types[a]
	if !ok {
		return false
	}
	tb, ok := pass.TypesInfo.Types[b]
	if !ok {
		return false
	}
	return types.Identical(ta.Type, tb.Type)
}
