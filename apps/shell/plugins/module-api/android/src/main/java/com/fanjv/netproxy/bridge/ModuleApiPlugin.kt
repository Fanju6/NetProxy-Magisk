package com.fanjv.netproxy.bridge

import android.app.Activity
import android.content.ContextWrapper
import android.content.res.Configuration
import android.webkit.WebView
import androidx.appcompat.app.AppCompatActivity
import androidx.core.view.WindowCompat
import androidx.lifecycle.Lifecycle
import app.tauri.annotation.Command
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
class NodeTargetArgs {
    var requestId: String = ""
    var target: Map<String, Any?>? = null
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

    // Signatures verified against tauri 2.12.4 mobile/android Plugin.kt.
    override fun load(webView: WebView) {
        val candidate = viewActivity(webView) ?: lifecycle.owner ?: return
        if (!attachOwner(candidate)) return
        super.load(webView)
        applyAppearance()
        val owner = lifecycle.owner as? AppCompatActivity
        bridge.load(webView, owner?.lifecycle?.currentState?.isAtLeast(Lifecycle.State.RESUMED) == true)
    }

    override fun onPause(activity: AppCompatActivity) {
        if (!lifecycle.pause(activity)) return
        publishVisibility()
        super.onPause(activity)
    }

    override fun onResume(activity: AppCompatActivity) {
        if (!attachOwner(activity)) return
        lifecycle.resume(activity, replaceOwner = false)
        super.onResume(activity)
        applyAppearance()
        publishVisibility()
    }

    override fun onConfigurationChanged(newConfig: Configuration) {
        super.onConfigurationChanged(newConfig)
        applyAppearance()
    }

    override fun onDestroy(activity: AppCompatActivity) {
        if (!lifecycle.destroy(activity)) return
        super.onDestroy(activity)
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
    fun selectNode(invoke: Invoke) = dispatch(invoke) {
        val args = invoke.parseArgs(NodeTargetArgs::class.java)
        val result = bridge.selectNode(args.requestId, args.target)
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
    fun testDelay(invoke: Invoke) = dispatch(invoke) {
        val args = invoke.parseArgs(NodeTargetArgs::class.java)
        val result = bridge.testDelay(args.requestId, args.target)
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
        if (args.event != "visibility") throw BridgeFailure("api.invalid_request")
        lifecycle.registerVisibility(args.handler.id) { visible ->
            args.handler.send(JSObject().put("visible", visible))
        }
        null
    }

    @Command
    override fun removeListener(invoke: Invoke) = immediate(invoke) {
        val args = invoke.parseArgs(RemoveVisibilityListenerArgs::class.java)
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
            result.fold({ invoke.resolve(it) }, { reject(invoke, it) })
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
