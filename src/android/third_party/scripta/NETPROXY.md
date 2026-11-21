# Scripta Source Snapshot

This directory vendors the editor module from [YuKongA/scripta](https://github.com/YuKongA/scripta).

- Upstream base: `e9c3da3d8ff077a286568f5faba926a15c525370`
- NetProxy snapshot: `f16f424603cdba0233d2e04a80e483fcc0f7568b`
- License: Apache License 2.0, preserved in [LICENSE](LICENSE)

The NetProxy snapshot adds the mobile completion popup, symbol toolbar improvements, controller completion APIs, and related editor tests used by the Android configuration editor. It is vendored because the NetProxy snapshot is not available from the upstream Git repository and a submodule reference would therefore be impossible to initialize.

Local NetProxy changes adapt the Desktop IME request to the current Compose text, selection, and composing-region APIs. The Android Gradle Plugin version is kept aligned with the manager and MIUIX composite builds. Android Host and Desktop editor tests are included in local and CI Android verification.

The editor accepts an optional top content inset and scroll callback for NetProxy's Glass header. The first line starts below the header, and fixed editor controls stay below it. An optional bottom content inset limits caret reveal and extends the scroll range so the last line can clear a floating host toolbar without shortening the drawing viewport.

When updating this copy, import a complete reviewed source snapshot, preserve the upstream license, record the new base revision here, and run the Android editor and application tests.
