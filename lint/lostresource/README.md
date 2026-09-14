# lostresource

`lostresource` is a golangci-lint module plugin that detects dropped resource
handles. It matches explicitly configured result types using `go/types`, including
aliases, generic instantiations, and tuple positions. Implementing `Release` or
`Close` alone does not make a type a resource.

It reports both discarded results and a control-flow path that returns or
overwrites a handle without releasing it or transferring it. The path
search uses `ctrlflow.Analyzer` and `go/cfg`, the same analysis infrastructure as
Go's `lostcancel` check. Reading a handle does not release it.

The analyzer also exports `go/analysis` facts for return guarantees proved from
reachable CFG returns. It skips results that are always nil and prunes error or
absence branches when the callee proves no handle can exist there. Forwarding
functions share these facts across packages; local forwarding chains converge
independently of declaration order. Interface dispatch, unknown implementations,
and named results changed by defers need explicit contracts or cleanup.

```go
handle, err := acquire() // possible leak: the error return skips release
if err != nil {
    return err
}
defer release(handle)
```

For APIs that can return partial resources with an error, register cleanup before
checking the error, using the API's nil-safe release helper. If the acquisition
contract guarantees that an error means no resource, configure `nil-on-error`.

## Installation

Add the module to `.custom-gcl.yml`, selecting a published common commit as its
version. Common's own build uses the local `path` form below:

```yaml
version: v2.13.2
plugins:
  - module: github.com/aperturerobotics/common/lint/lostresource
    path: ./lint/lostresource
```

Build with `golangci-lint custom` or the existing `aptre lint` integration. In a
consumer repository, replace `path` with `version: <common commit>`.

Enable the linter and specify resource contracts in `.golangci.yml`:

```yaml
version: "2"
linters:
  enable:
    - lostresource
  settings:
    custom:
      lostresource:
        type: module
        description: Checks discarded handles and missing resource releases.
        settings:
          check-paths: true
          resources:
            - type: github.com/s4wave/spacewave/db/world.ObjectState
              consumers:
                - github.com/s4wave/spacewave/db/world.ReleaseObjectState:0
            - type: github.com/s4wave/spacewave/bldr/resource/client.ResourceRef
              release-methods: [Release]
              borrowed:
                - github.com/s4wave/spacewave/sdk/root.Root.GetResourceRef
                - github.com/s4wave/spacewave/sdk/session.Session.GetResourceRef
                - github.com/s4wave/spacewave/sdk/world.Engine.GetResourceRef
                - github.com/s4wave/spacewave/sdk/world.WorldState.GetResourceRef
                - github.com/s4wave/spacewave/sdk/world.ObjectState.GetResourceRef
                - github.com/s4wave/spacewave/sdk/viewer/registry.ViewerRegistry.GetResourceRef
                - github.com/s4wave/spacewave/sdk/provider/local.LocalProvider.GetResourceRef
                - github.com/s4wave/spacewave/sdk/provider/spacewave.SpacewaveProvider.GetResourceRef
```

Contracts are opt-in; at least one resource entry is required. Audit production
code and test fixtures before enforcement. Set `check-paths: false` to roll out
only the discarded-result check first. The default is `true`.

## Contracts

| Setting | Meaning |
| --- | --- |
| `type` | Exact `import/path.Type`; prefix with `*` for a pointer result. |
| `release-methods` | Methods that consume the acquired receiver. |
| `consumers` | Cleanup or transfer functions, written `import/path.Function:0` or `import/path.Receiver.Method:0`. The zero-based position excludes a method receiver. |
| `success-consumers` | Same syntax, but consumes only when the final returned error is nil. The caller must check that error and release on failure. |
| `borrowed` | Fully qualified functions or methods returning borrowed values of this type. |
| `nil-on-error` | Explicit guarantee that a non-nil final error implies no acquired handle. Defaults to false. |
| `nil-on-error-functions` | Fully qualified acquisition functions with that guarantee, without applying it to every result of the type. |
| `nil-on-false-functions` | Acquisition functions whose penultimate boolean is false only when no handle is returned. |

A consumer only discharges its configured argument. Add each consumed argument
separately when a function takes several handles. The argument may also be a
callback proven to release the handle, when that API owns the callback's cleanup
obligation. Cleanup wrappers and adopting
constructors need these contracts; arbitrary function calls are treated as
borrowing. Use a body-only lookup when only the decoded value is needed.

Assignments to `_`, variable declarations, conditional initializers, standalone
calls, and discarded results in `go` and `defer` calls are checked. Returning a
call or passing its results directly to another call is outside the named-variable
path search; that receiving call must honor its resource contract.

## Path analysis and limits

The analysis follows local aliases, parallel assignments, overwrites, branches,
loops, named returns, direct cleanup calls, method expressions, receiver defers,
parameterless invoked or deferred literals, local cleanup callbacks, and
`testing.T/B/F.Cleanup`. Returning a callback that releases the handle on every
returning path transfers its cleanup obligation. Bound release methods capture
their original receiver even when its variable is subsequently overwritten.
An unused cleanup closure does not count as cleanup. Deferred closures read their
captured variables at return; receiver defers capture the handle immediately.
Diagnostics point to the acquisition and include the line of the return or
overwrite that loses it. Keeping both in one diagnostic lets a deliberate
`nolint:lostresource` exception suppress the entire finding.

Returning a handle, storing it in a field or aggregate, sending it on a channel,
or passing it to a configured consumer transfers the obligation. The receiving
component's eventual cleanup is not proven. Cleanup callbacks stored in aggregate
literals or appended to cleanup collections follow the same transfer rule.
Tracking cleanup after aggregate storage, pointer aliases,
indirect cleanup calls, arbitrary closure protocols, and interprocedural
transfer require further analysis. Configure explicit transfer wrappers for those
APIs. Consumers must accept the handle on every return path. Configure constructors
that adopt only on success under `success-consumers`; their error branch retains
the caller's obligation. An ignored or overwritten error cannot prove adoption.
Cleanup registered before acquisition, asynchronous captured-variable changes,
and mixed test-cleanup/defer lifetimes are not fully modeled.

Nil comparisons on the current handle are pruned, as are checks of its unchanged
acquisition error and found result under inferred or configured guarantees.
Boolean combinations preserve those known conditions. Other correlations may produce
false positives. The CFG has no general short-circuit or panic/recover model;
the check conservatively credits only the first short-circuit operand and follows
`ctrlflow` for nonreturning calls. Infinite paths without an overwrite or return
are not reported. This is a useful leak detector, not proof of resource safety;
retain runtime resource-count tests.

Run its fixtures independently of common's root module:

```sh
cd lint/lostresource
go test ./...
```
