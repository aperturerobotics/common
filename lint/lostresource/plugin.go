// Package lostresource checks discarded handles and paths that lose a call
// result before releasing it or transferring it to another component.
package lostresource

import (
	"go/ast"
	"go/types"
	"slices"
	"strconv"
	"strings"

	"github.com/golangci/plugin-module-register/register"
	"github.com/pkg/errors"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/ctrlflow"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// init registers the module with golangci-lint's plugin loader.
func init() {
	register.Plugin("lostresource", New)
}

// Plugin holds the resource contracts used by one configured linter.
type Plugin struct {
	// Resources specifies result types; no structural contract is inferred.
	Resources []Resource `json:"resources"`
	// CheckPaths enables control-flow checks in addition to discarded results.
	// An omitted value enables both checks.
	CheckPaths *bool `json:"check-paths"`
}

// New constructs a module plugin from golangci-lint settings.
func New(settings any) (register.LinterPlugin, error) {
	p, err := register.DecodeSettings[Plugin](settings)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// GetLoadMode requests the resolved types needed for exact type matching.
func (p *Plugin) GetLoadMode() string {
	return register.LoadModeTypesInfo
}

// BuildAnalyzers validates the contracts and constructs the analysis pass.
func (p *Plugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	// Reject ambiguous contracts before inspecting any source files.
	if len(p.Resources) == 0 {
		return nil, errors.New("lostresource requires at least one resources entry")
	}
	seen := make(map[string]bool)
	for _, r := range p.Resources {
		if !strings.Contains(r.Type, ".") || seen[r.Type] {
			return nil, errors.Errorf("lostresource: invalid or duplicate type %q", r.Type)
		}
		seen[r.Type] = true
		if len(r.ReleaseMethods)+len(r.Consumers) == 0 {
			return nil, errors.Errorf("lostresource: %s needs release-methods or consumers", r.Type)
		}
		for _, consumer := range r.Consumers {
			name, arg, ok := strings.Cut(consumer, ":")
			index, err := strconv.Atoi(arg)
			if !ok || !strings.Contains(name, ".") || err != nil || index < 0 {
				return nil, errors.Errorf("lostresource: invalid consumer %q; use package.Function:argument", consumer)
			}
		}
	}

	// Reuse the same typed CFG producer used by the lostcancel analyzer.
	return []*analysis.Analyzer{{
		Name:     "lostresource",
		Doc:      "checks discarded handles and paths missing release or transfer",
		Requires: []*analysis.Analyzer{inspect.Analyzer, ctrlflow.Analyzer},
		Run:      p.run,
	}}, nil
}

// run checks result positions and then follows locally acquired handles.
func (p *Plugin) run(pass *analysis.Pass) (any, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	insp.Preorder([]ast.Node{(*ast.AssignStmt)(nil), (*ast.ValueSpec)(nil), (*ast.ExprStmt)(nil), (*ast.GoStmt)(nil), (*ast.DeferStmt)(nil)}, func(node ast.Node) {
		lhs, rhs := bindings(node)
		for i, expr := range rhs {
			call, ok := ast.Unparen(expr).(*ast.CallExpr)
			if !ok || pass.TypesInfo.Types[call.Fun].IsType() {
				continue
			}
			name := functionName(pass.TypesInfo, call)
			resultTypes := []types.Type{pass.TypesInfo.TypeOf(call)}
			if tuple, ok := resultTypes[0].(*types.Tuple); ok {
				resultTypes = nil
				for v := range tuple.Variables() {
					resultTypes = append(resultTypes, v.Type())
				}
			}
			for j, typ := range resultTypes {
				for _, r := range p.Resources {
					if typ == nil || typeName(typ) != r.Type || slices.Contains(r.Borrowed, name) {
						continue
					}
					var target ast.Expr
					if i+j < len(lhs) {
						target = lhs[i+j]
					}
					id, _ := target.(*ast.Ident)
					if target == nil || id != nil && id.Name == "_" {
						where := ast.Node(call)
						if id != nil {
							where = id
						}
						pass.ReportRangef(where, "%s result from %s is discarded; release with %s or transfer it", r.Type, name, r.cleanup())
						continue
					}
					if id == nil || p.CheckPaths != nil && !*p.CheckPaths {
						continue
					}
					v, _ := pass.TypesInfo.ObjectOf(id).(*types.Var)
					if v == nil {
						continue
					}
					NewFlow(pass, r).Check(node, call, v)
				}
			}
		}
	})
	return nil, nil
}

// bindings exposes assignment positions, including calls with discarded tuples.
func bindings(node ast.Node) (lhs, rhs []ast.Expr) {
	switch n := node.(type) {
	case *ast.AssignStmt:
		return n.Lhs, n.Rhs
	case *ast.ValueSpec:
		for _, id := range n.Names {
			lhs = append(lhs, id)
		}
		return lhs, n.Values
	case *ast.ExprStmt:
		return nil, []ast.Expr{n.X}
	case *ast.GoStmt:
		return nil, []ast.Expr{n.Call}
	case *ast.DeferStmt:
		return nil, []ast.Expr{n.Call}
	}
	return nil, nil
}

// _ is a type assertion
var _ register.LinterPlugin = ((*Plugin)(nil))
