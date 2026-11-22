package com.fanjv.netproxy.feature.proxy.data

import android.content.SharedPreferences
import kotlinx.serialization.Serializable
import kotlinx.serialization.SerializationException
import kotlinx.serialization.json.Json

@Serializable
internal data class PolicyDelays(
    val coreIdentity: Long = 0,
    val groups: Map<String, Map<String, Int>> = emptyMap(),
)

internal class PolicyDelayStore(
    private val read: () -> String?,
    private val write: (String) -> Unit,
) {
    constructor(preferences: SharedPreferences) : this(
        { preferences.getString("history", null) },
        { preferences.edit().putString("history", it).apply() },
    )

    fun load(): PolicyDelays = try {
        read()?.let { Json.decodeFromString<PolicyDelays>(it) } ?: PolicyDelays()
    } catch (_: SerializationException) {
        PolicyDelays()
    }

    fun save(delays: PolicyDelays) {
        write(Json.encodeToString(delays))
    }
}
