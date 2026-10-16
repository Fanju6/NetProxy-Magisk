package com.fanjv.netproxy.bridge

import android.app.Activity
import android.webkit.WebView
import androidx.appcompat.app.AppCompatActivity
import androidx.lifecycle.Lifecycle
import app.tauri.annotation.Command
import app.tauri.annotation.InvokeArg
import app.tauri.annotation.TauriPlugin
import app.tauri.plugin.Invoke
import app.tauri.plugin.JSObject
import app.tauri.plugin.Plugin
import com.fanjv.netproxy.core.api.ConnectionStore
import com.fanjv.netproxy.core.api.LocalHttpTransport
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.CoroutineStart
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import org.json.JSONObject

@InvokeArg
class ReadArgs {
    var requestId: String = ""
    var resource: String = ""
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

@TauriPlugin
class ModuleApiPlugin(activity: Activity) : Plugin(activity) {
    private var ownerActivity: Activity? = activity
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val bridge = ModuleApiBridge(
        LocalHttpTransport(ConnectionStore(activity.applicationContext), allowProvisioning = false), scope,
    )

    // Signatures verified against tauri 2.12.4 mobile/android Plugin.kt.
    override fun load(webView: WebView) {
        super.load(webView)
        val owner = ownerActivity as? AppCompatActivity
        bridge.load(webView, owner?.lifecycle?.currentState?.isAtLeast(Lifecycle.State.RESUMED) == true)
    }

    override fun onPause(activity: AppCompatActivity) {
        if (activity !== ownerActivity) return
        bridge.pause()
        trigger("visibility", JSObject().put("visible", false))
        super.onPause(activity)
    }

    override fun onResume(activity: AppCompatActivity) {
        if (activity !== ownerActivity) return
        super.onResume(activity)
        bridge.resume()
        trigger("visibility", JSObject().put("visible", bridge.visible))
    }

    override fun onDestroy(activity: AppCompatActivity) {
        if (activity !== ownerActivity) return
        ownerActivity = null
        bridge.destroy()
        super.onDestroy(activity)
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

    private fun immediate(invoke: Invoke, action: () -> JSObject?) {
        try {
            invoke.resolve(action())
        } catch (error: Exception) {
            reject(invoke, error)
        }
    }

    private fun dispatch(invoke: Invoke, action: suspend () -> JSObject) {
        // Enter the try block before dispatch: even cancellation before IO starts must settle Invoke.
        scope.launch(start = CoroutineStart.UNDISPATCHED) {
            try {
                currentCoroutineContext().ensureActive()
                val result = withContext(Dispatchers.IO) { action() }
                currentCoroutineContext().ensureActive()
                invoke.resolve(result)
            } catch (error: Exception) {
                reject(invoke, error)
            }
        }
    }

    private fun reject(invoke: Invoke, error: Exception) {
        val safe = safeFailure(error)
        // Do not pass exceptions to Invoke.reject: Tauri logs them, including nested URLs.
        invoke.reject(SAFE_FAILURE_MESSAGE, safe.code, null, JSObject().put("status", safe.status))
    }
}
