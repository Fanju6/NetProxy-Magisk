# V8 Android Compatibility: Read-Only Tauri PoC

Status: investigation draft, 2026-10-16. This is not permission to install an
APK, migrate app data, or enable module writes. Existing V8 source and root
`AGENTS.md` remain authoritative. No credentials or signing material belong here.

## Identity and Manifest

V8 facts come from `src/android/app/build.gradle.kts` and
`src/android/app/src/main/AndroidManifest.xml`:

| Contract | V8 value / migration requirement |
| --- | --- |
| Application ID and namespace | `com.fanjv.netproxy`; an in-place replacement must keep the application ID. |
| Version code | `git rev-list --count HEAD`; the local fallback of `1` is not a safe release strategy. |
| Version name | Gradle property `netproxyManagerVersion`, otherwise module.prop version without `v`; CI appends `-ci.<build-id>`. |
| SDK / ABI | minSdk 26, targetSdk/compileSdk 37, arm64-v8a. Inspect the final APK, not only Tauri config. |
| Launcher | `com.fanjv.netproxy.MainActivity`, exported; MAIN + LAUNCHER. |
| Tile preferences entry | Existing MainActivity accepts `android.service.quicksettings.action.QS_TILE_PREFERENCES`. |
| Credentials Provider | `com.fanjv.netproxy.core.api.ConnectionProvider`, authority `com.fanjv.netproxy.connection`, not exported. |
| FileProvider | `androidx.core.content.FileProvider`, authority `com.fanjv.netproxy.fileprovider`, not exported, grants URI permissions. |
| Quick Settings service | `com.fanjv.netproxy.NetProxyTileService`, exported only with `android.permission.BIND_QUICK_SETTINGS_TILE`; QS_TILE action. |
| Permissions | INTERNET and ACCESS_NETWORK_STATE. No Root permission or shell fallback in the app. |
| Backup | `android:allowBackup="false"`. Do not relax this for testing. |

Do not ship the scaffold's temporary identifier, version `0.1.0`, or a debug
application-ID suffix as a V8 replacement. A side-by-side debug package cannot
read V8's sandbox and is not the manager accepted by the module helper.

Keep `.MainActivity` as the new Tauri entry class, or retain that component as an
activity alias to the new launcher. Do not create duplicate launcher entries.
V8 MainActivity has no explicit custom deep-link or extra-handling implementation;
do not invent an entry migration protocol.

The V8 `NetProxyApplication` initializes Compose dependencies and preferences.
Do not point a Tauri manifest at that class without satisfying its initialization
contract. In particular, the old TileService casts `application` to that class.
Copying only its manifest declaration would leave a runtime crash.

## Private Storage

Paths below are user-0 examples. Resolve them through Android Context at runtime,
not hard-coded absolute paths. A same-package signed update preserves the sandbox;
uninstall, clear-data, another application ID, or another Android user does not.

| Source | Storage contract |
| --- | --- |
| `core/api/ModuleConnection.kt` | `Context.noBackupFilesDir/module-connection.json`, normally `/data/user/0/com.fanjv.netproxy/no_backup/module-connection.json`; atomic writes and process-shared cache. |
| `NetProxyApplication.kt`, `core/di/AppContainer.kt`, `feature/theme/presentation/ThemePreferenceStore.kt` | SharedPreferences `settings`, normally `shared_prefs/settings.xml`. |
| `feature/catalog/presentation/nodes/list/CatalogNodesScreen.kt` | SharedPreferences `node_display`, normally `shared_prefs/node_display.xml`; layout_style and density, with an existing columns fallback. |
| `feature/logs/data/LogRepository.kt` | `Context.cacheDir/reports/`; report shares survive for one day, with cleanup on the next export. |
| `res/xml/file_paths.xml` | Only cache path `reports/`, exposed with logical name `log_reports`. Never expose the data root or no_backup. |

Theme preferences include color_mode, miuix_monet, key_color, color_style,
color_spec, enable_blur, enable_predictive_back, enable_smooth_corner and
page_scale. Preserve the files even when the PoC does not consume these settings.
Do not migrate connection material into WebView localStorage, JS state, logs,
URLs, browser storage, or general Tauri store plugins.

There is no V8 device-protected connection-store path in the reviewed source.
The existence of `/data/user_de/0/com.fanjv.netproxy` does not establish that
credentials exist there. Before first unlock, CE storage can be inaccessible.

## Provisioning and Read-Only Boundaries

`src/android/platform/src/main/java/com/fanjv/netproxy/platform/Main.java`
defines MANAGER_PACKAGE, MANAGER_CERTIFICATE_SHA256 and the fixed authority. Its
`verifyManager(PackageManager)` requires user 0, one package per UID, exactly one
current signer matching that fingerprint, and the expected non-exported Provider
owned by the installed manager UID. Delivery rechecks the bound Provider.

`ConnectionProvider.call(method: String, arg: String?, extras: Bundle?): Bundle`
accepts only Binder UID 0, method `provision`, null arg and the existing delivery
format. Do not export it or add JS access to delivery contents.

The PoC must construct LocalHttpTransport with `allowProvisioning = false`.
The default remains true for unchanged V8 callers. A fresh sandbox without trusted
connection material must return an explicit unavailable error, not use the V8
unverified TLS provisioning bootstrap. This also applies to explicit reconnect.
Do not call `api access`, change module credentials, or trigger provisioning as
part of this read-only investigation.

Native commands should allowlist read operations rather than accept arbitrary
paths/methods. Never expose the native HTTP client's mutation methods through a
generic JS bridge. Read-only HTTPS transport tests do not prove bridge allowlists.

## Tauri Lifecycle APIs

Checked against the official **tauri-v2.12.4** tag, matching the shell's declared
version at investigation time. The upstream lockfile contains tauri-runtime-wry
2.12.2, wry 0.57.0 and tao 0.37.0. These are upstream reference versions, not a
claim about the shell's eventual Cargo.lock; recheck the resolved build.

Official source:

- [Plugin.kt](https://github.com/tauri-apps/tauri/blob/tauri-v2.12.4/crates/tauri/mobile/android/src/main/java/app/tauri/plugin/Plugin.kt)
- [PluginManager.kt](https://github.com/tauri-apps/tauri/blob/tauri-v2.12.4/crates/tauri/mobile/android/src/main/java/app/tauri/plugin/PluginManager.kt)
- [TauriActivity.kt template](https://github.com/tauri-apps/tauri/blob/tauri-v2.12.4/crates/tauri/mobile/android-codegen/TauriActivity.kt)
- [WryActivity.kt](https://github.com/tauri-apps/wry/blob/wry-v0.57.0/src/android/kotlin/WryActivity.kt)
- [wry Android main_pipe.rs](https://github.com/tauri-apps/wry/blob/wry-v0.57.0/src/android/main_pipe.rs)
- [wry Android mod.rs](https://github.com/tauri-apps/wry/blob/wry-v0.57.0/src/android/mod.rs)

The plugin extension points are:

```kotlin
open fun load(webView: android.webkit.WebView)
open fun onPause(activity: androidx.appcompat.app.AppCompatActivity)
open fun onResume(activity: androidx.appcompat.app.AppCompatActivity)
open fun onDestroy(activity: androidx.appcompat.app.AppCompatActivity)
```

The corresponding no-argument lifecycle methods still exist but are deprecated.
Internal triggerOnPause/triggerOnResume/triggerOnDestroy call both the Activity
overload and then the deprecated overload. Override one variant, not both.
The plugin onResume documentation includes cold starts; do not assume plugin
initialization always precedes the first resume. A watch request must also be
able to establish its foreground session after initialization.

TauriActivity calls its WryActivity superclass, then PluginManager. The latter
dispatches callbacks to every registered plugin with the originating Activity.
Account for ownership if more than one Activity exists. Do not globally close
another Activity's active session in response to an unrelated callback.

There is **no Plugin.onWebViewDestroy callback** in this version. load(WebView)
and WryActivity.onWebViewCreate(WebView) are creation hooks. WryActivity's
onDestroy calls Rust.onDestroy(this), then Rust.onWebviewDestroy(this, id).
Only after returning does TauriActivity call PluginManager.onDestroy(this).
The Rust pipe skips callback-map cleanup for configuration-change recreation;
otherwise `destroy_webview(activity_id: ActivityId, webview_id: &WebviewId)`
removes its registered Rust handlers. This function is not an Android
`WebView.destroy()` invocation.

[Android WebView.destroy()](https://developer.android.com/reference/android/webkit/WebView#destroy())
is a separate API: detach from the view system first, call on the creating UI
thread, and make no further WebView calls afterward. The reviewed WryActivity,
RustWebView and pipe teardown do not establish an explicit Java destroy call.
The plugin does not own Tauri's WebView; do not manually destroy it, replace its
WebViewClient, or equate view detachment with permanent destruction.

Recommended PoC ownership: stop native SSE/read jobs on pause, recreate a
foreground scope on resume when there are subscribers, and make destruction
cleanup idempotent. Drop channel/WebView references and suppress callbacks from
the old session. Do not rely solely on JS visibility events or assume onDestroy
runs before process death. A per-WebView close protocol beyond Activity teardown
needs explicit shell ownership and verification, not a fabricated plugin hook.

## Reusing V8 Security Tests

The plugin can compile the existing `core/api` Kotlin sources directly into its
own module. Keep their Kotlin package names: Gradle namespace is independent of
those packages. Do not depend on the Compose application project or duplicate
internal transport types in a second module.

For the library at `apps/shell/plugins/module-api/android`, the repository root
is `projectDir.resolve("../../../../..")`. The useful production source directory
is `src/android/app/src/main/java/com/fanjv/netproxy/core/api`. It contains the
transport, ConnectionStore, codec, event frames, client and private Provider.
The corresponding test directory is
`src/android/app/src/test/java/com/fanjv/netproxy/core/api`.

Add these directories to the Android/Kotlin main and test source sets, keeping
the plugin's own sources. Initially exclude ModuleApiClientTest.kt and
ModuleEventStreamTest.kt from the imported test directory, not from production.
The directly reusable baseline is:

- LocalHttpTransportTest: 36 tests for TLS identity/hostname/date handling,
  redirects, proxies, retry/cancellation boundaries, credential replacement,
  downloads, size limits and SSE capacity/disconnection.
- ModuleConnectionTest: 7 tests for strict addresses/material, caching, atomic
  replacement visibility and stale-failure protection.
- ModuleApiCodecTest: 10 tests for schema/types, HTTP/domain failures, streaming
  decode and frame memoization.
- TestApiTransport.kt: test helper required by ModuleApiCodecTest.

Match V8's Kotlin/serialization compiler and runtime versions rather than the
scaffold defaults. At review time these are Kotlin 2.4.20,
kotlinx-serialization-json 1.11.0, OkHttp and mockwebserver3 5.5.0, JUnit 4.13.2,
and kotlinx-coroutines-test 1.9.0. Add the Kotlin serialization compiler plugin,
coroutines runtime, and the listed test dependencies. Confirm dependency
resolution rather than silently upgrading versions in this investigation.
Keep a real JDK with keytool: the transport tests generate disposable anonymous
TLS identities under build/tmp and remove them. Do not replace them with real
module credentials or APK signing keys. Use a suitable host test JDK for the
codec's ThreadMXBean allocation checks.

From the generated Android host, run its actual plugin project's
`testDebugUnitTest` with `--tests` filters for the three classes above. Determine
that project name from settings/`gradlew projects`; it is not necessarily the
Cargo plugin name. `--tests` filters execution, not Kotlin compilation, so they
do not remove the two repository-dependent suites from the test compile inputs.
Those suites import SubscriptionRepository and/or ServiceRepository and need
their real dependency closure before reuse. Do not introduce fake repositories
just to claim the full V8 suite passed.

Separately add plugin-specific tests for fresh/no-provision failure, reconnect
without provisioning, GET-only allowlists, lifecycle pause/resume/destroy,
stale-session suppression and bounded native channel delivery. This document
does not implement them or claim they pass.

## Fixed Signing and Overwrite Safety

The maintained signing entry is `.github/actions/verify-android/action.yml`,
step "use fixed key to sign manager" (Chinese step name in source).
It builds the V8 release APK, validates package/version metadata, signs using
GitHub-provided material via environment references, verifies the APK, and
compares its certificate against MANAGER_CERTIFICATE_SHA256 in platform Main.java.
It writes the resulting manager to `src/module/NetProxy.apk`.

Callers are `.github/workflows/build-module.yml`, `.github/workflows/ci.yml`
and `.github/workflows/release.yml`. `.github/scripts/build-metadata.sh` requires
non-shallow history and produces the shared version/commit count. There is no
standalone maintained local signing script in the reviewed paths. Do not run the
whole Action as a Tauri signing helper: it builds V8 and overwrites a module
artifact. Reuse its verification procedure with an explicitly chosen shell APK,
and keep the output outside module packaging until separately authorized.

Official references: [Android signing](https://developer.android.com/studio/publish/app-signing),
[ADB package manager](https://developer.android.com/tools/adb#pm).

Before any separately authorized overwrite:

1. Unlock user 0 normally, then confirm CE data and the no_backup receipt are
   accessible. A locked/missing path is not evidence of an empty sandbox.
2. Save every installed APK/split and verify its package, version and signature.
   Prepare a fixed-signed V8 rollback APK whose versionCode is at least the PoC's.
   Raising PoC's code can make the pulled old APK insufficient for rollback.
3. Arrange an approved encrypted/private backup location outside this repository
   for app data. Preserve CE and any relevant DE data, ownership, modes and
   SELinux-label information. Do not copy connection material into `.agents/`,
   exported storage, diagnostic bundles, terminal output or this document.
   `adb backup` is not a substitute for no_backup data and allowBackup=false.
4. Verify the final PoC APK has exactly the expected current signing certificate,
   application ID and intended versionCode. Tauri's default signing or a new
   debug key is not compatible. Signing lineage accepted by Android alone would
   not satisfy the helper's exact current-certificate check.
5. Only after explicit installation authorization use a replace/retain-data
   install. Never uninstall first, clear data, enable backup, request downgrade
   as a shortcut, or relax Provider/TLS checks after a mismatch. Abort instead.
6. Verify receipt continuity without printing its contents, read-only transport,
   navigation, lifecycle/SSE cleanup, and an actual signed rollback rehearsal.

## Tile and Sharing Migration Gates

Keep the Tile component identity if claiming V8 tile continuity, but do not
enable its old start/stop click behavior in a read-only PoC. The old service
depends on the V8 application container; it cannot be transplanted unchanged.
Either explicitly defer/disable Tile support or implement an authorized
read-only replacement. Removing the service must be recorded as a compatibility
gap, not a silent successful migration. Verify tile-preferences navigation.

Retain the FileProvider authority and the reports-only XML restriction if
claiming diagnostic sharing. Do not substitute a generic Tauri filesystem/share
plugin with broader directory access. Existing grant lifetimes and receiver
behavior require device validation. If sharing is out of scope, preserve old
data and record the unavailable entry rather than implementing a write path.

## Verification Recorded in This Investigation

- V8 baseline command: from `src/android`, `gradlew.bat :app:testDebugUnitTest`
  with `--tests` filters for LocalHttpTransportTest, ModuleConnectionTest and
  ModuleApiCodecTest, `--console=plain --no-daemon`: **53 tests, zero failures,
  errors or skips**. This is the V8 baseline, not a new-plugin Gradle run.
- ADB read-only checks found user 0 RUNNING_LOCKED. DE app directory exists;
  the CE receipt cannot be verified until normal unlock. No app-data backup or
  credential read was performed.
- Installed V8 APK: package com.fanjv.netproxy, code 1085, name
  8.6.0-ci.4784efe; local HEAD count was 1091. APK pulled to an ignored research
  run and verified with apksigner: one signer, v2/v3 signatures valid, fixed
  certificate matches platform Main.java. This old-code APK alone is not a
  rollback for a code-1091 PoC.
- Expected CI signing environment variables were not present in this process.
  No signing material was read, no APK was signed or installed, no device data
  was changed by investigation commands, and no commit was created.
- Downloaded tagged official source and the non-sensitive APK are under
  `.agents/runs/2026-10-16-tauri-android-research-e7c4/`. Recheck live device and
  resolved dependency state before using these dated observations.
