# polling

`polling` is a golangci-lint module plugin that rejects timers which poll for a
state change. The component that holds a piece of state should publish its
changes through an event, stream, broadcast or revision, and its consumers
should watch that. A timer that rereads the state instead adds up to one period
of latency, wakes when nothing changed, and hides the missing watch.

The analyzer reports `time.Sleep`, `time.NewTicker` and `time.Tick` anywhere,
and `time.After` inside a loop. It skips test files and generated files, and a
function literal starts outside any loop. A one-shot `time.After`,
`time.NewTimer` and `time.AfterFunc` are not reported.

A timer that waits for a known time, not for a state change, carries a
directive on its line or the line above, optionally followed by a reason:

```go
//gostyle:timer backoff
case <-time.After(delay):
```

| Kind       | Use                                                        |
| ---------- | ---------------------------------------------------------- |
| `deadline` | a known expiry, interval or pause                          |
| `backoff`  | the delay between retries after a failure                  |
| `external` | a bounded poll of an outside system that offers no watch   |

A directive with any other kind is reported. Enable the plugin in a project's
`.custom-gcl.yml`:

```yaml
plugins:
  - module: 'github.com/aperturerobotics/common/lint/polling'
    version: <pseudo-version>
```

and in `.golangci.yml` under `linters.settings.custom`:

```yaml
polling:
  type: 'module'
  description: Rejects timers that poll for a state change in place of a watch.
```
