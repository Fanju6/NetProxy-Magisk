package com.fanjv.netproxy.feature.proxy.data

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject

internal val editablePolicyTypes = setOf("selector", "urltest", "loadbalance")
internal const val AUTO_POLICY_TAG = "Auto"

internal data class PolicyGroupDraft(
    val tag: String = "",
    val type: String = "selector",
    val outbounds: List<String> = emptyList(),
    val providers: List<String> = emptyList(),
    val useAll: Boolean = false,
    val defaultMember: String = "",
    val include: String = "",
    val exclude: String = "",
    val testUrl: String = "",
    val testInterval: String = "",
    val addAuto: Boolean = false,
) {
    val hasMembers: Boolean get() = outbounds.isNotEmpty() || providers.isNotEmpty() || useAll || addAuto

    companion object {
        fun from(group: JsonObject) = PolicyGroupDraft(
            tag = group.text("tag"),
            type = group.text("type"),
            outbounds = group.names("outbounds"),
            providers = group.names("providers"),
            useAll = (group["use_all_providers"] as? JsonPrimitive)?.booleanOrNull == true,
            defaultMember = group.text("default"),
            include = group.text("include"),
            exclude = group.text("exclude"),
            testUrl = group.text("url"),
            testInterval = group.text("interval"),
        )
    }
}

internal data class PolicyChoice(val tag: String, val label: String = tag)

internal data class PolicyGroupDocument(
    val outbounds: List<JsonObject>,
    val revision: String,
    val originalTag: String?,
    val memberTags: List<String>,
    val providerChoices: List<PolicyChoice>,
) {
    private val index = originalTag?.let { tag ->
        outbounds.indexOfFirst { it.text("tag") == tag }.also { require(it >= 0) }
    }
    private val original = index?.let(outbounds::get)
    val initial = original?.let(PolicyGroupDraft::from) ?: PolicyGroupDraft()
    val canAddAuto: Boolean get() = originalTag == "Proxy" && initial.type == "selector" &&
        (initial.outbounds.isNotEmpty() || initial.defaultMember.isNotBlank()) &&
        AUTO_POLICY_TAG !in memberTags && AUTO_POLICY_TAG !in initial.outbounds &&
        outbounds.any { it.text("tag") == "block" && it.text("type") == "block" }

    fun updated(draft: PolicyGroupDraft): String {
        val tag = originalTag ?: draft.tag.trim()
        require(tag.isNotBlank() && draft.hasMembers && tag !in draft.outbounds)
        require(draft.type in editablePolicyTypes)
        require(originalTag == null || (draft.tag == originalTag && draft.type == initial.type))
        require(index != null || tag !in memberTags)
        require(!draft.addAuto || canAddAuto)
        val fields = original.orEmpty().toMutableMap()
        fields["tag"] = JsonPrimitive(tag)
        fields["type"] = JsonPrimitive(draft.type)
        fun updateText(key: String, value: String, previous: String) {
            if (value != previous) {
                if (value.isBlank()) fields.remove(key) else fields[key] = JsonPrimitive(value)
            }
        }
        fun updateNames(key: String, value: List<String>, previous: List<String>) {
            if (value != previous) {
                if (value.isEmpty()) fields.remove(key) else fields[key] = JsonArray(value.map(::JsonPrimitive))
            }
        }
        val members = if (draft.addAuto) (draft.outbounds + AUTO_POLICY_TAG).distinct() else draft.outbounds
        updateNames("outbounds", members, initial.outbounds)
        updateNames("providers", draft.providers, initial.providers)
        if (draft.useAll != initial.useAll) {
            if (draft.useAll) fields["use_all_providers"] = JsonPrimitive(true) else fields.remove("use_all_providers")
        }
        if (draft.type == "selector") updateText("default", draft.defaultMember, initial.defaultMember)
        if (draft.type == "urltest") {
            updateText("url", draft.testUrl, initial.testUrl)
            updateText("interval", draft.testInterval, initial.testInterval)
        }
        updateText("include", draft.include, initial.include)
        updateText("exclude", draft.exclude, initial.exclude)
        val updated = outbounds.toMutableList()
        if (index == null) updated.add(JsonObject(fields)) else updated[index] = JsonObject(fields)
        if (draft.addAuto) updated.add(JsonObject(mapOf(
            "type" to JsonPrimitive("urltest"),
            "tag" to JsonPrimitive(AUTO_POLICY_TAG),
            "outbounds" to JsonArray(listOf(JsonPrimitive("block"))),
            "use_all_providers" to JsonPrimitive(true),
        )))
        return JsonObject(mapOf("outbounds" to JsonArray(updated))).toString()
    }
}

internal fun parsePolicySection(content: String, key: String): List<JsonObject> =
    when (val value = Json.parseToJsonElement(content).jsonObject[key]) {
        null, JsonNull -> emptyList()
        else -> value.jsonArray.map(JsonElement::jsonObject)
    }

internal fun JsonObject.text(key: String): String = (get(key) as? JsonPrimitive)?.contentOrNull.orEmpty()

private fun JsonObject.names(key: String): List<String> = when (val field = get(key)) {
    is JsonArray -> field.mapNotNull { (it as? JsonPrimitive)?.contentOrNull }
    is JsonPrimitive -> listOfNotNull(field.contentOrNull)
    else -> emptyList()
}
