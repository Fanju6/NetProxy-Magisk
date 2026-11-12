package com.fanjv.netproxy.feature.proxy.data

import com.fanjv.netproxy.core.api.ModuleApiClient
import com.fanjv.netproxy.feature.catalog.model.CatalogGroupSummary
import com.fanjv.netproxy.feature.settings.data.ConfigRepository
import kotlinx.coroutines.flow.map
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.Transient
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put

@Serializable
internal data class PolicySnapshot(
    val groups: List<PolicyGroup>,
    val live: Boolean,
    @SerialName("core_started_at_millis") val coreIdentity: Long
)

@Serializable
internal data class PolicyGroup(
    val tag: String,
    val type: String,
    val selectable: Boolean,
    val items: List<PolicyMember>,
    val selected: String = "",
    @SerialName("configured_default") val configuredDefault: String = "",
    @SerialName("unavailable_providers") val unavailableProviders: List<String> = emptyList()
)

@Serializable
internal data class PolicyMember(
    val tag: String,
    val type: String,
    @SerialName("url_test_delay") val delay: Int = 0,
    @SerialName("url_test_time") val delayTime: Long = 0,
    @Transient val runtimeAvailable: Boolean = false,
)

internal fun PolicySnapshot.withRuntime(
    runtime: PolicySnapshot?,
    showDelay: (String, PolicyMember) -> Boolean = { _, _ -> true },
): PolicySnapshot {
    if (runtime == null || !runtime.live || runtime.coreIdentity <= 0) return this
    val liveGroups = runtime.groups.associateBy { it.tag }
    return copy(
        live = true,
        coreIdentity = runtime.coreIdentity,
        groups = groups.map { configured ->
            val live = liveGroups[configured.tag]?.takeIf { it.type.equals(configured.type, ignoreCase = true) }
                ?: return@map configured
            val members = live.items.associateBy { it.tag }
            configured.copy(
                selectable = live.selectable,
                selected = live.selected.takeIf { tag -> tag in members && configured.items.any { it.tag == tag } }.orEmpty(),
                items = configured.items.map { member ->
                    val active = members[member.tag]
                    member.copy(
                        delay = active?.takeIf { it.delay > 0 && showDelay(configured.tag, it) }?.delay ?: 0,
                        runtimeAvailable = active != null,
                    )
                },
            )
        },
    )
}

internal class PolicyRepository(
    private val client: ModuleApiClient,
    private val config: ConfigRepository = ConfigRepository(client),
) {
    suspend fun openEditor(tag: String?): PolicyGroupDocument {
        val snapshot = config.readSnapshot("singbox/outbounds")
        val outbounds = parsePolicySection(snapshot.content, "outbounds")
        val endpoints = parsePolicySection(config.read("singbox/endpoints"), "endpoints")
        val nativeProviders = parsePolicySection(config.read("singbox/providers"), "providers")
            .map { PolicyChoice(it.text("tag")) }
        val resources = client.read<List<CatalogGroupSummary>>("/subscriptions")
            .filter { it.nodeCount > 0 }
            .map { PolicyChoice(it.runtimeTag, "${it.name} · ${it.runtimeTag}") }
        return PolicyGroupDocument(
            outbounds, snapshot.revision, tag,
            (outbounds + endpoints).map { it.text("tag") }.filter(String::isNotBlank).distinct(),
            (nativeProviders + resources).filter { it.tag.isNotBlank() }.distinctBy { it.tag },
        )
    }

    suspend fun saveGroup(document: PolicyGroupDocument, draft: PolicyGroupDraft) {
        config.apply("singbox/outbounds", document.updated(draft), document.revision)
    }

    suspend fun snapshot(): PolicySnapshot = client.read<PolicySnapshot>("/policy-groups", mapOf("source" to "config"))
        .also { check(!it.live && it.coreIdentity == 0L) }

    // Runtime frames supply status only; persisted config owns the displayed membership.
    fun changes() = client.observe(setOf("groups")).map { result ->
        result.mapCatching { it.getValue("groups").decode<PolicySnapshot>() }
    }

    suspend fun select(identity: Long, group: String, member: String): PolicySnapshot =
        client.json.decodeFromJsonElement(PolicySnapshot.serializer(),
            client.post("/policy-groups/select", buildJsonObject {
                put("core_started_at_millis", identity)
                put("group_tag", group)
                put("member_tag", member)
            }).data)

    suspend fun delay(identity: Long, group: String) {
        client.post("/policy-groups/delay", buildJsonObject {
            put("core_started_at_millis", identity)
            put("group_tag", group)
        })
    }
}
