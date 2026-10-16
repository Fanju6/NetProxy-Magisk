# Tauri Plugin module-api

Internal read-only Android bridge for NetProxy. Uses the official Tauri mobile
plugin mechanism; it is not a generic HTTP, filesystem or shell plugin.

The maintained scope, build instructions and credential boundary are documented
in [the shell README](../../README.md). Kotlin directly compiles the existing
V8 security transport sources rather than maintaining a copy.
