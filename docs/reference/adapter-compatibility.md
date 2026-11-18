# Compatibility imports

Existing platform versions can keep using these aliases. New code should use `contract/` directly.

| Compatibility import suffix | Canonical import suffix |
| --- | --- |
| `spine/adapter` | `spine/contract/http` |
| `spine/adapter/http` | `spine/contract/http` |
| `spine/adapter/event` | `spine/contract/event` |
| `spine/adapter/websocket` | `spine/contract/websocket` |

All paths begin with `github.com/NARUBROWN/`. Types, contract versions, errors and validation helpers preserve their identity. The former `spine/wsadapter` package was removed; migrate it to `spine/contract/websocket` or `spine/adapter/websocket`.

Compatibility aliases do not adapt incompatible contract versions. In particular, an HTTP v2 platform does not become an HTTP v3 platform by changing its import path. See [platform contracts](contracts.md).

[Korean original](../ko/reference/adapter-compatibility.md).
