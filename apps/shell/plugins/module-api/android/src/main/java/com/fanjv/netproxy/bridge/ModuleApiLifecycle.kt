package com.fanjv.netproxy.bridge

/** Tauri 跨 Activity 重建保留插件，并向所有插件广播各 Activity 的生命周期。 */
internal class ModuleApiLifecycle<Owner : Any>(
    owner: Owner,
    private val createBridge: () -> ModuleApiBridge,
) {
    var owner: Owner? = owner
        private set
    var bridge = createBridge()
        private set
    private var visibility: Pair<Long, (Boolean) -> Unit>? = null

    fun attach(candidate: Owner, replaceOwner: Boolean): Boolean {
        if (candidate !== owner) {
            if (!replaceOwner) return false
            bridge.destroy()
            // destroy 后新文档可能已注册，不能在迟到的 resume 中丢掉该回调。
            if (owner != null) visibility = null
            owner = candidate
            bridge = createBridge()
        }
        return true
    }

    fun resume(candidate: Owner, replaceOwner: Boolean): Boolean {
        if (!attach(candidate, replaceOwner)) return false
        bridge.resume()
        return true
    }

    fun pause(candidate: Owner): Boolean {
        if (candidate !== owner) return false
        bridge.pause()
        return true
    }

    fun destroy(candidate: Owner): Boolean {
        if (candidate !== owner) return false
        owner = null
        visibility = null
        bridge.destroy()
        return true
    }

    fun registerVisibility(id: Long, publish: (Boolean) -> Unit) {
        visibility = id to publish
    }

    fun removeVisibility(id: Long) {
        if (visibility?.first == id) visibility = null
    }

    fun publishVisibility() {
        // WebView 可能已经释放原生回调；发布失败不能中断 Activity 生命周期收尾。
        runCatching { visibility?.second?.invoke(bridge.visible) }
    }
}
