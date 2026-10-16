# Android Module API Plugin

`com.fanjv.netproxy.bridge.ModuleApiPlugin` implements the Tauri 2 Android plugin.
Only `read` (`resource: info | service`) and read-only SSE are available. The
frontend owns reconnection. Native resume only publishes visibility; it does not
reopen a stream or retry a request.

The six files in V8's `src/android/app/src/main/java/com/fanjv/netproxy/core/api`
are compiled in this module through Gradle source sets, not copied. The plugin
uses `ConnectionStore(activity.applicationContext)` and `LocalHttpTransport` with
`allowProvisioning = false`. A missing or invalid private credential delivery is
an explicit failure; the plugin never contacts the unauthenticated provision
endpoint, executes shell commands, or exposes an address/secret to JavaScript.

The IPC methods are `read`, `cancel`, `openEvents`, `nextEvent`, `closeEvents`, and
`visibility`. HTTP responses return `{status, body}` with a schema-1 envelope.
Business failures preserve HTTP status and code but replace message and data with
fixed safe values. Invalid envelopes reject instead of becoming successful IPC
results. SSE validates every envelope, including status-topic traffic frames.
Errors passed to `Invoke.reject` never include an exception or remote message.

There is one native subscription, one pending pull, and a `Channel(1)` queue.
Close and pause discard buffered frames, wake the pending pull, and cancel the
corresponding transport coroutine/OkHttp call. Destroy also cancels the plugin
scope. Ordinary read concurrency is capped at eight. No native reconnect loop or
`ModuleApiClient` instance is created.

## Build Integration

The generated app supplies the AGP/Kotlin/serialization compiler versions. This
module does not pin or upgrade them. The shell configuration is AGP 9.4.1 / Gradle 9.8.0,
Kotlin/serialization compiler 2.4.20, compile SDK 37, min SDK 26, and JVM target 17.
The generated root must include `org.jetbrains.kotlin:kotlin-serialization` at the
same version as its Kotlin Gradle plugin and disable AGP's built-in Kotlin when
applying `org.jetbrains.kotlin.android`, as the official generator does.

For standalone verification with an existing official Tauri 2.12.4 Android source
checkout (no Rust/app build required):

```text
gradle testDebugUnitTest assembleDebug lintDebug \
  -Pnetproxy.agpVersion=9.4.1 -Pnetproxy.kotlinVersion=2.4.20 \
  -Ptauri.androidPath=<absolute-path-to-tauri/mobile/android> \
  -Pandroid.builtInKotlin=false -Pandroid.newDsl=false -Pandroid.useAndroidX=true
```

Cancel-before-read is retained for 60 seconds in at most 128 tombstones. Overflow
rejects new reads for the grace window rather than evicting cancellations. A
replacement WebView cancels old work without destroying the system-owned view.

Plugin tests cover read-only admission, bounded work, codec checks, safe errors,
close/cancel races, backpressure, lifecycle, and missing credentials. Existing V8
TLS/store/codec tests run separately in `src/android`; they are not copied into
this module. Run the integrated plugin checks from `src-tauri/gen/android`:

```text
gradlew :tauri-plugin-module-api:testDebugUnitTest :tauri-plugin-module-api:lintDebug
```

## Official Lifecycle And Listeners

Signatures were checked against Tauri 2.12.4's `mobile/android/.../Plugin.kt`:
`load(WebView)`, `onPause(AppCompatActivity)`, `onResume(AppCompatActivity)`, and
`onDestroy(AppCompatActivity)`. The deprecated no-argument callbacks are not
overridden. Activity callbacks are checked against the plugin's owning activity;
late load uses the owner's lifecycle state. No WebView reference is retained or
destroyed by this plugin.

`registerListener` / `removeListener` are inherited official annotated commands.
`PluginHandle.indexMethods` walks superclasses. Tauri's mobile IPC fallback in
`src/webview/mod.rs` converts snake_case command names to lowerCamelCase, so the
default `allow-register-listener` / `allow-remove-listener` permissions cover
`addPluginListener('module-api', 'visibility', ...)` without additional commands.
