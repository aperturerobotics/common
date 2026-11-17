// Package polling provides a golangci-lint module plugin that rejects timers
// polling for a state change. A component that holds state publishes its
// changes through an event, stream, broadcast or revision, and a consumer
// watches it; a timer that rereads the state instead adds latency, wakes
// without cause and hides the missing watch.
//
// The analyzer reports time.Sleep, time.NewTicker and time.Tick anywhere, and
// time.After inside a loop, in non-test, non-generated files. A timer that
// waits for a known time carries a directive on its line or the line above:
//
//	//gostyle:timer deadline   a known expiry, interval or pause
//	//gostyle:timer backoff    the delay between retries after a failure
//	//gostyle:timer external   a bounded poll of a system with no watch
//
// Text after the kind is a free-form reason. Any other kind is reported.
package polling

import (
	"go/ast"
	"go/types"
	"strings"

	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/types/typeutil"
)

// timerDirective marks a timer that waits for a known time and not for a
// state change. The word after it names the kind of wait.
const timerDirective = "//gostyle:timer"

// timerKinds names the waits a timer directive may claim. A deadline waits
// for a known expiry, an interval or a pause; a backoff spaces retries after a
// failure; an external timer bounds a poll of a system outside the process that
// offers no watch, such as a file lock or a process group.
var timerKinds = map[string]bool{
	"deadline": true,
	"backoff":  true,
	"external": true,
}

// pollingRepair is the diagnostic's instruction: the cause to fix and the one
// marked exception.
const pollingRepair = "watch the component that holds the state through its event, stream, broadcast or revision instead; " +
	"mark a real deadline, backoff or external-system timer with " + timerDirective + " deadline|backoff|external on or above its line"

// init registers the module with golangci-lint's plugin loader.
func init() {
	register.Plugin("polling", New)
}

// plugin is the golangci-lint module plugin; it takes no settings.
type plugin struct{}

// New constructs the module plugin from golangci-lint settings.
func New(any) (register.LinterPlugin, error) {
	return &plugin{}, nil
}

// BuildAnalyzers returns the polling analysis pass.
func (p *plugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{NewAnalyzer()}, nil
}

// GetLoadMode returns the load mode; resolving time functions needs types.
func (p *plugin) GetLoadMode() string {
	return register.LoadModeTypesInfo
}

// NewAnalyzer checks for timers that wait for a state change by
// polling: time.Sleep, time.NewTicker and time.Tick anywhere, and time.After
// inside a loop. A timer marked with the timer directive and a known kind is
// accepted on its line or the line below. It omits generated files and tests,
// and it does not follow loops through called functions.
func NewAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name: "polling",
		Doc:  "rejects timers that poll for a state change in place of a watch",
		Run:  runPolling,
	}
}

// runPolling walks each production file, tracking whether a loop of the
// current function encloses each call.
func runPolling(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		// Tests may pace themselves; the contract covers production code.
		if !handwritten(pass, file) {
			continue
		}
		v := &pollingVisitor{pass: pass, marks: timerMarks(pass, file)}
		ast.Walk(v, file)
	}
	return nil, nil
}

// pollingVisitor visits nodes inside loops loops deep in the current function.
// marks maps each line holding a timer directive to the kind it names.
type pollingVisitor struct {
	pass  *analysis.Pass
	marks map[int]string
	loops int
}

// Visit reports a polling timer, walks loop bodies one deeper, and starts each
// function literal outside any loop, since its body runs as its own function.
func (v *pollingVisitor) Visit(node ast.Node) ast.Visitor {
	switch node := node.(type) {
	case *ast.FuncLit:
		ast.Walk(&pollingVisitor{pass: v.pass, marks: v.marks}, node.Body)
		return nil
	case *ast.ForStmt:
		walkAll(v, node.Init, node.Cond, node.Post)
		ast.Walk(v.deeper(), node.Body)
		return nil
	case *ast.RangeStmt:
		walkAll(v, node.Key, node.Value, node.X)
		ast.Walk(v.deeper(), node.Body)
		return nil
	case *ast.CallExpr:
		v.check(node)
	}
	return v
}

// deeper returns the visitor for a loop body.
func (v *pollingVisitor) deeper() *pollingVisitor {
	return &pollingVisitor{pass: v.pass, marks: v.marks, loops: v.loops + 1}
}

// check reports call when it is a polling timer without a valid directive.
func (v *pollingVisitor) check(call *ast.CallExpr) {
	// Select the time functions that poll.
	name := timeFunc(v.pass, call)
	switch name {
	case "Sleep", "NewTicker", "Tick":
	case "After":
		if v.loops == 0 {
			return
		}
	default:
		return
	}

	// Accept a directive on the call's line or the line above it.
	line := v.pass.Fset.Position(call.Pos()).Line
	kind, marked := v.marks[line]
	if !marked {
		kind, marked = v.marks[line-1]
	}
	if marked && timerKinds[kind] {
		return
	}
	if marked {
		v.pass.Reportf(call.Pos(), "%s timer kind %q is not deadline, backoff or external", timerDirective, kind)
		return
	}
	where := ""
	if name == "After" {
		where = " inside a loop"
	}
	v.pass.Reportf(call.Pos(), "time.%s%s polls for a state change; %s", name, where, pollingRepair)
}

// timerMarks maps each line of file that holds a timer directive to the kind
// the directive names, or "" when it names none.
func timerMarks(pass *analysis.Pass, file *ast.File) map[int]string {
	marks := make(map[int]string)
	for _, group := range file.Comments {
		for _, comment := range group.List {
			rest, ok := strings.CutPrefix(comment.Text, timerDirective)
			if !ok || (rest != "" && rest[0] != ' ') {
				continue
			}
			kind, _, _ := strings.Cut(strings.TrimSpace(rest), " ")
			marks[pass.Fset.Position(comment.Pos()).Line] = kind
		}
	}
	return marks
}

// timeFunc returns the name of the time package function call invokes, or ""
// for any other call.
func timeFunc(pass *analysis.Pass, call *ast.CallExpr) string {
	fn, ok := typeutil.Callee(pass.TypesInfo, call).(*types.Func)
	if !ok || fn.Pkg() == nil || fn.Pkg().Path() != "time" {
		return ""
	}
	if sig, ok := fn.Type().(*types.Signature); !ok || sig.Recv() != nil {
		return ""
	}
	return fn.Name()
}

// handwritten reports whether file is a non-generated, non-test source file.
func handwritten(pass *analysis.Pass, file *ast.File) bool {
	return !ast.IsGenerated(file) && !strings.HasSuffix(pass.Fset.Position(file.Pos()).Filename, "_test.go")
}

// walkAll walks each node that is present. An absent loop clause is a nil
// interface, which stays nil when converted to ast.Node.
func walkAll(v ast.Visitor, nodes ...ast.Node) {
	for _, node := range nodes {
		if node != nil {
			ast.Walk(v, node)
		}
	}
}
