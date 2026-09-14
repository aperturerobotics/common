package lostresource

import (
	"go/ast"
	"go/types"
	"slices"
	"strings"

	"golang.org/x/tools/go/types/typeutil"
)

// Resource describes a call result of one exact declared type that must be
// released or transferred.
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
	// SuccessConsumers consume their argument only when their final error is nil.
	// Entries use the same function:argument syntax as Consumers.
	SuccessConsumers []string `json:"success-consumers"`
	// Borrowed excludes results of these fully qualified functions or methods.
	Borrowed []string `json:"borrowed"`
	// NilOnError asserts that a non-nil final error means no handle was acquired.
	// Leave false for APIs that can return a partially acquired handle and error.
	NilOnError bool `json:"nil-on-error"`
	// NilOnErrorFunctions gives the same guarantee for selected acquisition APIs.
	NilOnErrorFunctions []string `json:"nil-on-error-functions"`
	// NilOnFalseFunctions guarantees no resource when the penultimate bool is false.
	NilOnFalseFunctions []string `json:"nil-on-false-functions"`
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
	if len(apis) == 0 && len(r.Consumers) != 0 {
		name, _, _ := strings.Cut(r.Consumers[0], ":")
		apis = append(apis, name)
	}
	return strings.Join(apis, " or ")
}
