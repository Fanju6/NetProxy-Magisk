# Tauri Plugin module-api

Internal restricted Android bridge for NetProxy. Uses the official Tauri mobile
plugin mechanism; it is not a generic HTTP, filesystem or shell plugin.

The maintained scope, build instructions and credential boundary are documented
in [the shell README](../../README.md). Kotlin directly compiles the existing
V8 security transport sources rather than maintaining a copy.

M4.3.2 Stage A adds only `subscription_action` with the flat IPC arguments
`{ requestId, groupId, action: "update" | "cancel" }`. Rust and Kotlin reject
unknown fields and invalid IDs independently. Both actions send a fixed POST
with an empty JSON body, without caller-controlled paths, queries or headers.
Updates share the existing business-write slot and 615-second transport budget;
server cancellation has its own single slot and a 30-second client wait bound.
The overall 10-dispatch limit remains in place; saturated dispatches fail busy
rather than queueing unbounded work. Success and error results expose only
validated identity, count, persistence and runtime-sync facts. Cancelling an
IPC wait does not send a server cancellation or prove that a transaction rolled
back. No action is automatically retried or replayed after lifecycle changes.

Stage B additionally exposes `subscription_write`, a flat tagged payload:

- `add`: `requestId`, `input` (HTTP(S) `url` required).
- `edit`: `requestId`, `groupId`, `input`.
- `remove`: `requestId`, `groupId`, optional `replacementGroupId`.
- `update_all`: `requestId` only.

All business writes share the existing write slot. Kotlin alone constructs
POST `/subscriptions`, PUT/DELETE `/subscriptions/{id}`, POST
`/subscriptions/update-all` and the optional `replacement_group_id` query.
Input keys follow the WebUI subscription schema; omission preserves a value,
while an empty string or header object explicitly clears supported fields.
Explicit nulls, unknown fields, invalid types, unsafe IDs, control characters,
invalid header tokens and out-of-bounds numbers are rejected independently in
Rust and Kotlin. Header values may contain horizontal tabs, matching the input
schema, but never CR/LF or other control characters.

User-entered subscription credentials temporarily cross JS and IPC as write-only
input, never returned through private GET, cached, logged or echoed in errors.
The module Bearer and certificate remain native-only. Update-all responses retain
strictly validated `updated`/`failed` ID arrays and a single projected
`first_error`; raw exception text and nested private metadata are dropped.
