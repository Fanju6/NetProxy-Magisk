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
- `remove`: `requestId`, `groupId` only.
- `update_all`: `requestId` only.

All business writes share the existing write slot. Kotlin alone constructs
POST `/subscriptions`, PUT/DELETE `/subscriptions/{id}`, POST
`/subscriptions/update-all`. Deletion never sends a replacement-group query.
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

## Native Policy Groups

- `read({ requestId, resource: "policy_groups" })`: GET `/policy-groups`.
- `policy_select({ requestId, groupTag, memberTag })`: POST
  `/policy-groups/select` with `{ group_tag, member_tag }`.
- `policy_delay({ requestId, groupTag })`: POST `/policy-groups/delay` with
  `{ group_tag }`.

Rust and Kotlin independently reject unknown fields, wrong types, blank tags,
control characters and requests above 4 MiB. Tags retain their Unicode and
spacing; malformed Unicode is rejected. Tags never enter a URL or query.
Both POSTs share the existing business-write slot and never retry or replay.

All three responses and SSE `groups` use the same public snapshot projection:
`{ groups: serviceapi.Group[], live, core_started_at_millis, default_outbound }`.
Only public group and member fields cross IPC; credentials, private metadata,
unknown extensions and original exception messages are discarded. Error results
retain HTTP status and business code; error data retains only boolean
`accepted`/`persisted`/`runtime_synced` and a nonnegative integer
`core_started_at_millis`, when present.

`nodes` remains GET `/nodes/snapshot`; `catalog` uses GET `/catalog`. Both return
`{ groups: [{ group, nodes }] }`. Catalog groups expose `provider_tag`, not
`runtime_tag` or `active`. Service status GET, action responses and SSE `status`
use a public-field whitelist including `default_outbound` and `runtime_selected`;
the former single-selection fields are absent. Mode and service controls remain
unchanged. There is no `selection` resource, `select_node` or `test_delay`
permission or command. Native state remains owned by each plugin lifecycle.

Local resource groups use the existing `management` permission and strict
operations: `catalog_create` with `input: { id, name }`, `catalog_rename` with
`input: { id, name }`, and `catalog_remove` with `input: { id }`. Kotlin alone
constructs POST `/catalog`, PUT `/catalog/{id}` with `{ name }`, or DELETE
`/catalog/{id}`. IDs follow the existing Catalog ID rules; deleting `default`
is rejected in both Rust and Kotlin. Names must be nonblank, valid UTF-8, at
most 256 bytes and free of control characters. Successful responses expose
only the matching `{ id }`; group type and transaction checks remain authoritative
on the Go server. No additional command, scope or caller-controlled path is added.
