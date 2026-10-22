package com.fanjv.netproxy.bridge

import android.app.Activity
import android.content.ContextWrapper
import android.content.Intent
import android.content.res.Configuration
import android.net.Uri
import android.os.Build
import android.webkit.WebView
import androidx.activity.result.ActivityResult
import androidx.appcompat.app.AppCompatActivity
import androidx.core.view.WindowCompat
import androidx.lifecycle.Lifecycle
import app.tauri.annotation.Command
import app.tauri.annotation.ActivityCallback
import app.tauri.annotation.InvokeArg
import app.tauri.annotation.TauriPlugin
import app.tauri.plugin.Invoke
import app.tauri.plugin.Channel
import app.tauri.plugin.JSObject
import app.tauri.plugin.Plugin
import com.fanjv.netproxy.core.api.ConnectionStore
import com.fanjv.netproxy.core.api.LocalHttpTransport
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.Job
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.cancel
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import java.lang.ref.WeakReference
import org.json.JSONObject

@InvokeArg
class ReadArgs {
    var requestId: String = ""
    var resource: String = ""
}

@InvokeArg
class SubscriptionReadArgs {
    var requestId: String = ""
    var groupId: String = ""
    var resource: String = ""
}

@InvokeArg
class ServiceActionArgs {
    var requestId: String = ""
    var action: String = ""
}

@InvokeArg
class ChangeModeArgs {
    var requestId: String = ""
    var mode: Any? = null
}

@InvokeArg
class CancelArgs {
    var requestId: String = ""
}

@InvokeArg
class EventsArgs {
    var subscriptionId: String = ""
    var topics: List<String> = emptyList()
}

@InvokeArg
class VisibilityListenerArgs {
    var event: String = ""
    lateinit var handler: Channel
}

@InvokeArg
class RemoveVisibilityListenerArgs {
    var event: String = ""
    var channelId: Long = 0
}

@TauriPlugin
class ModuleApiPlugin(activity: Activity) : Plugin(activity) {
    private val ownerClass = activity.javaClass
    private val appearance = SystemBarAppearance()
    private val transport = LocalHttpTransport(ConnectionStore(activity.applicationContext))
    private val lifecycle = ModuleApiLifecycle(activity) {
        ModuleApiBridge(
            transport,
            CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate),
            prepareDiagnostics = ::prepareDiagnostics,
        )
    }
    private val bridge get() = lifecycle.bridge
    private val documents = AndroidDocuments(activity.applicationContext)
    private val imports = ImportInbox()
    private var inputScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)
    private var inputView: WeakReference<WebView>? = null
    private var inputGeneration = 0L
    private var picker: Invoke? = null
    private var importListener: Pair<Long, Channel>? = null
    private val shareReads = HashSet<Job>()
    private val shareReadOrder = Mutex()

    // Signatures verified against tauri 2.12.4 mobile/android Plugin.kt.
    override fun load(webView: WebView) {
        val candidate = viewActivity(webView) ?: lifecycle.owner ?: return
        if (!attachOwner(candidate)) return
        if (inputView?.get() !== webView) {
            if (inputView != null) releaseInputWork()
            inputView = WeakReference(webView)
        }
        super.load(webView)
        applyAppearance()
        val owner = lifecycle.owner as? AppCompatActivity
        bridge.load(webView, owner?.lifecycle?.currentState?.isAtLeast(Lifecycle.State.RESUMED) == true)
        acceptShare(candidate.intent)
    }

    override fun onPause(activity: AppCompatActivity) {
        if (!lifecycle.pause(activity)) return
        publishVisibility()
        publishImport()
        super.onPause(activity)
    }

    override fun onResume(activity: AppCompatActivity) {
        if (!attachOwner(activity)) return
        lifecycle.resume(activity, replaceOwner = false)
        super.onResume(activity)
        applyAppearance()
        publishVisibility()
        if (inputView?.get() != null) acceptShare(activity.intent)
        publishImport()
    }

    override fun onConfigurationChanged(newConfig: Configuration) {
        super.onConfigurationChanged(newConfig)
        applyAppearance()
    }

    override fun onDestroy(activity: AppCompatActivity) {
        if (!lifecycle.destroy(activity)) return
        releaseInputWork()
        inputView = null
        if (activity.isFinishing) imports.clear()
        super.onDestroy(activity)
    }

    override fun onNewIntent(intent: Intent) {
        if (inputView?.get() == null) lifecycle.owner?.intent = intent
        else acceptShare(intent)
    }

    private fun releaseInputWork() {
        inputGeneration++
        picker?.let { reject(it, BridgeFailure("api.destroyed")) }
        picker = null
        importListener = null
        inputScope.cancel()
        inputScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)
        shareReads.clear()
    }

    private fun contentResult(value: ImportContent) = JSObject().apply {
        put("id", value.id); put("name", value.name); put("content", value.content)
    }

    @Command
    fun takeImport(invoke: Invoke) = immediate(invoke) {
        val args = JSONObject(invoke.getRawArgs())
        if (args.length() == 0) {
            if (!bridge.visible) throw BridgeFailure("api.background")
            imports.take(inputGeneration)?.let(::contentResult)
        }
        else {
            val id = args.opt("completedId")
            if (args.length() != 1 || id !is String || id.length !in 1..96 || !id.matches(Regex("[A-Za-z0-9_-]+")))
                throw BridgeFailure("api.invalid_request")
            imports.complete(id)
            null
        }
    }

    @Command
    fun pickDocument(invoke: Invoke) {
        val owner = lifecycle.owner ?: return reject(invoke, BridgeFailure("api.destroyed"))
        owner.runOnUiThread {
            try {
                if (!bridge.visible || owner !== lifecycle.owner) throw BridgeFailure("api.background")
                val args = JSONObject(invoke.getRawArgs())
                val kind = args.opt("kind")
                if (args.length() != 1 || kind !in listOf("config", "nodes")) throw BridgeFailure("api.invalid_request")
                if (picker != null || imports.hasActive()) throw BridgeFailure("api.busy")
                picker = invoke
                val intent = Intent(Intent.ACTION_OPEN_DOCUMENT).apply {
                    addCategory(Intent.CATEGORY_OPENABLE)
                    type = "*/*"
                    putExtra(Intent.EXTRA_MIME_TYPES, if (kind == "config")
                        arrayOf("application/json", "text/*", "application/octet-stream")
                    else arrayOf("text/*", "application/json", "application/yaml", "application/x-yaml", "application/octet-stream"))
                    addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
                }
                startActivityForResult(invoke, intent, "documentResult")
            } catch (error: Exception) {
                if (picker === invoke) picker = null
                reject(invoke, error)
            }
        }
    }

    @ActivityCallback
    fun documentResult(invoke: Invoke, result: ActivityResult) {
        if (picker !== invoke) return
        if (result.resultCode != Activity.RESULT_OK) {
            picker = null
            invoke.resolve()
            return
        }
        val expected = inputGeneration
        inputScope.launch {
            try {
                val uri = result.data?.data ?: throw BridgeFailure("api.import_unreadable")
                val value = documents.read(uri)
                if (expected == inputGeneration && picker === invoke)
                    invoke.resolve(contentResult(imports.claim(value, inputGeneration)))
            } catch (error: Exception) {
                if (expected == inputGeneration && picker === invoke) reject(invoke, error)
            } finally { if (picker === invoke) picker = null }
        }
    }

    @Suppress("DEPRECATION")
    private fun acceptShare(intent: Intent?) {
        if (intent?.action != Intent.ACTION_SEND) return
        val input = runCatching {
            val uri = if (Build.VERSION.SDK_INT >= 33) intent.getParcelableExtra(Intent.EXTRA_STREAM, Uri::class.java)
                else intent.getParcelableExtra<Uri>(Intent.EXTRA_STREAM)
            if (intent.clipData?.itemCount?.let { it > 1 } == true) throw BridgeFailure("api.import_invalid")
            val stream = uri ?: intent.clipData?.takeIf { it.itemCount == 1 }?.getItemAt(0)?.uri
            val text = if (stream == null) intent.getCharSequenceExtra(Intent.EXTRA_TEXT)?.toString() else null
            stream to text
        }
        // Consume the external Intent before async IO, including Activity recreation.
        intent.action = Intent.ACTION_MAIN
        intent.replaceExtras(android.os.Bundle())
        intent.clipData = null
        intent.data = null
        if (shareReads.size >= 4) {
            imports.offer(Result.failure(BridgeFailure("api.busy"))); publishImport(); return
        }
        val job = inputScope.launch(start = kotlinx.coroutines.CoroutineStart.LAZY) {
            shareReadOrder.withLock {
                val value = runCatching {
                    val (uri, text) = input.getOrThrow()
                    if (uri != null) documents.read(uri)
                    else importContent("", text ?: throw BridgeFailure("api.import_invalid"))
                }
                if (value.exceptionOrNull() is CancellationException) return@withLock
                imports.offer(value.mapFailure())
                publishImport()
            }
        }
        shareReads.add(job)
        job.invokeOnCompletion { shareReads.remove(job) }
        job.start()
    }

    private fun Result<ImportContent>.mapFailure(): Result<ImportContent> = fold(
        { Result.success(it) }, { Result.failure(it as? BridgeFailure ?: BridgeFailure("api.import_unreadable")) })

    private fun publishImport() {
        runCatching { importListener?.second?.send(JSObject()) }
    }

    private fun attachOwner(activity: Activity): Boolean {
        if (activity.isDestroyed || activity.isFinishing) return false
        val owner = lifecycle.owner
        val replaceOwner = activity.javaClass == ownerClass &&
            (owner == null || owner.isDestroyed || owner.isChangingConfigurations)
        return lifecycle.attach(activity, replaceOwner)
    }

    private fun viewActivity(webView: WebView): Activity? {
        var context = webView.context
        repeat(16) {
            if (context is Activity) return context as Activity
            context = (context as? ContextWrapper)?.baseContext ?: return null
        }
        return null
    }

    @Command
    fun subscriptionRead(invoke: Invoke) = dispatch(invoke) {
        val args = invoke.parseArgs(SubscriptionReadArgs::class.java)
        val result = bridge.subscriptionRead(args.requestId, args.groupId, args.resource)
        JSObject().apply {
            put("status", result.status)
            put("body", JSONObject(result.body.toString()))
        }
    }

    @Command
    fun subscriptionAction(invoke: Invoke) = dispatch(invoke) {
        val result = bridge.subscriptionAction(invoke.getRawArgs())
        JSObject().apply {
            put("status", result.status)
            put("body", JSONObject(result.body.toString()))
        }
    }

    @Command
    fun management(invoke: Invoke) {
        bridge.dispatch({ bridge.management(invoke.getRawArgs()) }) { result ->
            result.fold({ response ->
                invoke.resolve(JSObject().apply {
                    put("status", response.status)
                    put("body", JSONObject(response.body.toString()))
                })
            }, { reject(invoke, it) })
        }
    }

    private suspend fun prepareDiagnostics(): DiagnosticShare.Prepared {
        val owner = lifecycle.owner ?: throw BridgeFailure("api.destroyed")
        return DiagnosticShare.prepare(owner, transport)
    }

    @Command
    fun subscriptionWrite(invoke: Invoke) = dispatch(invoke) {
        val result = bridge.subscriptionWrite(invoke.getRawArgs())
        JSObject().apply {
            put("status", result.status)
            put("body", JSONObject(result.body.toString()))
        }
    }

    @Command
    fun read(invoke: Invoke) = dispatch(invoke) {
        val args = invoke.parseArgs(ReadArgs::class.java)
        val result = bridge.read(args.requestId, args.resource)
        JSObject().apply {
            put("status", result.status)
            put("body", JSONObject(result.body.toString()))
        }
    }

    @Command
    fun serviceAction(invoke: Invoke) = dispatch(invoke) {
        val args = invoke.parseArgs(ServiceActionArgs::class.java)
        val result = bridge.serviceAction(args.requestId, args.action)
        JSObject().apply {
            put("status", result.status)
            put("body", JSONObject(result.body.toString()))
        }
    }

    @Command
    fun policySelect(invoke: Invoke) = dispatch(invoke) {
        val result = bridge.policySelect(invoke.getRawArgs())
        JSObject().apply {
            put("status", result.status)
            put("body", JSONObject(result.body.toString()))
        }
    }

    @Command
    fun changeMode(invoke: Invoke) = dispatch(invoke) {
        val args = invoke.parseArgs(ChangeModeArgs::class.java)
        val result = bridge.changeMode(args.requestId, args.mode)
        JSObject().apply {
            put("status", result.status)
            put("body", JSONObject(result.body.toString()))
        }
    }

    @Command
    fun policyDelay(invoke: Invoke) = dispatch(invoke) {
        val result = bridge.policyDelay(invoke.getRawArgs())
        JSObject().apply {
            put("status", result.status)
            put("body", JSONObject(result.body.toString()))
        }
    }

    @Command
    fun cancel(invoke: Invoke) = immediate(invoke) {
        bridge.cancel(invoke.parseArgs(CancelArgs::class.java).requestId)
        null
    }

    @Command
    fun openEvents(invoke: Invoke) = immediate(invoke) {
        val args = invoke.parseArgs(EventsArgs::class.java)
        bridge.openEvents(args.subscriptionId, args.topics)
        null
    }

    @Command
    fun nextEvent(invoke: Invoke) = dispatch(invoke) {
        val result = bridge.nextEvent(invoke.parseArgs(EventsArgs::class.java).subscriptionId)
        JSObject().apply {
            put("name", result.name)
            put("body", JSONObject(result.body.toString()))
        }
    }

    @Command
    fun closeEvents(invoke: Invoke) = immediate(invoke) {
        bridge.closeEvents(invoke.parseArgs(EventsArgs::class.java).subscriptionId)
        null
    }

    @Command
    fun visibility(invoke: Invoke) = immediate(invoke) { JSObject().put("visible", bridge.visible) }

    @Command
    fun setAppearance(invoke: Invoke) {
        val owner = lifecycle.owner
        if (owner == null) {
            reject(invoke, BridgeFailure("api.destroyed"))
            return
        }
        owner.runOnUiThread {
            immediate(invoke) {
                if (owner !== lifecycle.owner || owner.isDestroyed || owner.isFinishing) throw BridgeFailure("api.destroyed")
                appearance.update(invoke.getRawArgs())
                applyAppearance()
                null
            }
        }
    }

    private fun applyAppearance() {
        val owner = lifecycle.owner ?: return
        if (owner.isDestroyed || owner.isFinishing) return
        appearance.reapply { light ->
            WindowCompat.getInsetsController(owner.window, owner.window.decorView).apply {
                isAppearanceLightStatusBars = light
                isAppearanceLightNavigationBars = light
            }
        }
    }

    // 沿用官方 listener 协议，但新文档必须替换旧回调，不能累积已失联的 Channel。
    @Command
    override fun registerListener(invoke: Invoke) = immediate(invoke) {
        val args = invoke.parseArgs(VisibilityListenerArgs::class.java)
        if (args.event == "import") {
            importListener = args.handler.id to args.handler
            return@immediate null
        }
        if (args.event != "visibility") throw BridgeFailure("api.invalid_request")
        lifecycle.registerVisibility(args.handler.id) { visible ->
            args.handler.send(JSObject().put("visible", visible))
        }
        null
    }

    @Command
    override fun removeListener(invoke: Invoke) = immediate(invoke) {
        val args = invoke.parseArgs(RemoveVisibilityListenerArgs::class.java)
        if (args.event == "import") {
            if (importListener?.first == args.channelId) importListener = null
            return@immediate null
        }
        if (args.event != "visibility") throw BridgeFailure("api.invalid_request")
        lifecycle.removeVisibility(args.channelId)
        null
    }

    private fun publishVisibility() {
        lifecycle.publishVisibility()
    }

    private fun immediate(invoke: Invoke, action: () -> JSObject?) {
        val result = runCatching { action() }
        runCatching {
            result.fold({ if (it == null) invoke.resolve() else invoke.resolve(it) }, { reject(invoke, it) })
        }
    }

    private fun dispatch(invoke: Invoke, action: suspend () -> JSObject) {
        bridge.dispatch(action) { result ->
            result.fold({ invoke.resolve(it) }, { reject(invoke, it) })
        }
    }

    private fun reject(invoke: Invoke, error: Throwable) {
        val safe = safeFailure(error as? Exception ?: BridgeFailure("api.native_failed"))
        // Do not pass exceptions to Invoke.reject: Tauri logs them, including nested URLs.
        invoke.reject(SAFE_FAILURE_MESSAGE, safe.code, null, JSObject().put("status", safe.status))
    }
}
