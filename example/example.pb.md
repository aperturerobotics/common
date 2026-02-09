# github.com/aperturerobotics/common/example/example.proto

**Package:** `example`  
**Syntax:** `proto3`

## Messages

### ExampleMsg

ExampleMsg is an example message.

| Field | Type | Label | Description |
|-------|------|-------|-------------|
| `example_field` | `string` |  | ExampleField is an example field. |
| `other_msg` | `OtherMsg` |  | OtherMsg is an example of an imported message field. |

### EchoMsg

EchoMsg is the message body for Echo.

| Field | Type | Label | Description |
|-------|------|-------|-------------|
| `body` | `string` |  |  |

## Services

### Echoer

Echoer service returns the given message.

| Method | Request | Response | Description |
|--------|---------|----------|-------------|
| `Echo` | `EchoMsg` | `EchoMsg` | Echo returns the given message. |

