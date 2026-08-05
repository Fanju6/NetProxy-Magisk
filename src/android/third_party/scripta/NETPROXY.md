# Scripta Source Snapshot

This directory vendors the editor module from [YuKongA/scripta](https://github.com/YuKongA/scripta).

- Upstream base: `e9c3da3d8ff077a286568f5faba926a15c525370`
- NetProxy snapshot: `f16f424603cdba0233d2e04a80e483fcc0f7568b`
- License: Apache License 2.0, preserved in [LICENSE](LICENSE)

The NetProxy snapshot adds the mobile completion popup, symbol toolbar improvements, controller completion APIs, and related editor tests used by the Android configuration editor. It is vendored because the NetProxy snapshot is not available from the upstream Git repository and a submodule reference would therefore be impossible to initialize.

When updating this copy, import a complete reviewed source snapshot, preserve the upstream license, record the new base revision here, and run the Android editor and application tests.
