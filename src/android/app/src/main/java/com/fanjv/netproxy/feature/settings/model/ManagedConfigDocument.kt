package com.fanjv.netproxy.feature.settings.model

import kotlinx.serialization.Serializable

@Serializable
internal data class ManagedConfigDocument(
    val id: String,
    val filename: String,
    val category: String,
    val editable: Boolean,
    val section: String = ""
)

@Serializable
internal data class ConfigSnapshot(val content: String, val revision: String)
