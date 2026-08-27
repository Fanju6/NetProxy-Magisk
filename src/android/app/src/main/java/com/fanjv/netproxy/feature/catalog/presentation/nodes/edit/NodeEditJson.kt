package com.fanjv.netproxy.feature.catalog.presentation.nodes.edit

import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.contentOrNull

/** listableStrings 将 sing-box Listable 字段统一读取为字符串列表。 */
internal fun JsonElement?.listableStrings(): List<String> = when (this) {
    is JsonArray -> mapNotNull { (it as? JsonPrimitive)?.contentOrNull }
    is JsonPrimitive -> contentOrNull?.let(::listOf).orEmpty()
    else -> emptyList()
}
