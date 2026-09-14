package ownedresource

import (
	"go/ast"
	"go/types"
	"slices"
	"strings"

	"golang.org/x/tools/go/types/typeutil"
)

// Resource describes ownership of call results with one exact declared type.
type Resource struct {
	// Type is the declaring import path and type name, separated by a dot.
	// A leading * selects pointers instead of values of the declared type.
	Type string `json:"type"`
	// ReleaseMethods consume their receiver, such as Release or Close.
	ReleaseMethods []string `json:"release-methods"`
	// Consumers consume handle arguments, including cleanup and transfer APIs.
	// Each entry is an import path, function or receiver.method, and :argument.
	// Argument positions are zero-based and exclude a method's receiver.
	Consumers []string `json:"consumers"`
	// Borrowed excludes results of these fully qualified functions or methods.
	Borrowed []string `json:"borrowed"`
	// NilOnError asserts that a non-nil final error means no handle was acquired.
	// Leave false for APIs that can return a partially acquired handle and error.
	NilOnError bool `json:"nil-on-error"`
}

// typeName preserves declared identity across aliases and generic instantiations.
func typeName(t types.Type) string {
	// Unwrap aliases at both sides of a pointer without matching implementations.
	t = types.Unalias(t)
	prefix := ""
	if ptr, ok := t.(*types.Pointer); ok {
		prefix = "*"
		t = types.Unalias(ptr.Elem())
	}
	if named, ok := t.(*types.Named); ok && named.Obj().Pkg() != nil {
		return prefix + named.Obj().Pkg().Path() + "." + named.Obj().Name()
	}
	return ""
}

// functionName resolves import aliases, generic calls, and method expressions.
func functionName(info *types.Info, call *ast.CallExpr) string {
	fn := typeutil.Callee(info, call)
	if fn == nil || fn.Pkg() == nil {
		return types.ExprString(call.Fun)
	}
	if sig, ok := fn.Type().(*types.Signature); ok && sig.Recv() != nil {
		return strings.TrimPrefix(typeName(sig.Recv().Type()), "*") + "." + fn.Name()
	}
	return fn.Pkg().Path() + "." + fn.Name()
}

// cleanup describes the APIs that discharge this resource's obligation.
func (r Resource) cleanup() string {
	apis := slices.Clone(r.ReleaseMethods)
	for i := range apis {
		apis[i] += "()"
	}
	return strings.Join(append(apis, r.Consumers...), " or ")
}
