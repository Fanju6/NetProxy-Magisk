package com.fanjv.netproxy.bridge

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json

/** Only the WebUI's resolved appearance is retained across Activity recreation. */
internal class SystemBarAppearance {
    @Serializable
    enum class Theme {
        @SerialName("light") LIGHT,
        @SerialName("dark") DARK,
    }

    @Serializable
    private data class Request(val theme: Theme)

    private var theme: Theme? = null

    fun update(raw: String) {
        if (raw.toByteArray(Charsets.UTF_8).size > 128) throw BridgeFailure("api.invalid_request", 400)
        val request = try { Json.decodeFromString<Request>(raw) }
            catch (_: Exception) { throw BridgeFailure("api.invalid_request", 400) }
        theme = request.theme
    }

    fun reapply(apply: (light: Boolean) -> Unit) {
        theme?.let { apply(it == Theme.LIGHT) }
    }
}
