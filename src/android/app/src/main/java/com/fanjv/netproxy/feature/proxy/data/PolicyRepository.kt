package com.fanjv.netproxy.feature.proxy.data

import com.fanjv.netproxy.core.api.ModuleApiClient
import com.fanjv.netproxy.feature.catalog.model.CatalogGroupSummary
import com.fanjv.netproxy.feature.catalog.model.CatalogNodeGroup
import com.fanjv.netproxy.feature.settings.data.ConfigRepository
import kotlinx.coroutines.flow.map
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.Transient
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put

@Serializable
internal data class PolicySnapshot(
    val groups: List<PolicyGroup>,
    val live: Boolean,
    @SerialName("core_started_at_millis") val coreIdentity: Long,
    @SerialName("config_revision") val configRevision: String = "",
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
    @SerialName("display_name") val name: String = "",
    @SerialName("provider_tag") val providerTag: String = "",
)

internal fun PolicySnapshot.withRuntime(
    runtime: PolicySnapshot?,
    displayDelay: (String, PolicyMember) -> Int = { _, member -> member.delay.coerceAtLeast(0) },
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
                        delay = active?.let { displayDelay(configured.tag, it) } ?: 0,
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
    suspend fun openEditor(tag: String?): PolicyGroupDocument = groupDocument(tag, catalogGroups(), loadNodes = true)

    private suspend fun catalogGroups() = client.read<List<CatalogGroupSummary>>("/subscriptions")

    private suspend fun groupDocument(tag: String?, resources: List<CatalogGroupSummary>, loadNodes: Boolean = false): PolicyGroupDocument {
        val snapshot = config.readSnapshot("singbox/outbounds")
        val outbounds = parsePolicySection(snapshot.content, "outbounds")
        val endpoints = parsePolicySection(config.read("singbox/endpoints"), "endpoints")
        val nativeProviders = parsePolicySection(config.read("singbox/providers"), "providers")
            .map { it.text("tag") }
        val providerNodes = if (loadNodes && resources.any { it.nodeCount > 0 }) client.read<List<CatalogNodeGroup>>("/nodes")
            .filterNot { it.group.runtimeTag in nativeProviders }
            .associate { item -> item.group.runtimeTag to item.nodes.map { item.group.runtimeTag + "/" + it.tag } }
        else emptyMap()
        return PolicyGroupDocument(
            outbounds, snapshot.revision, tag,
            (outbounds + endpoints).map { it.text("tag") }.filter(String::isNotBlank).distinct(),
            (nativeProviders + resources.filter { it.nodeCount > 0 }.map { it.runtimeTag })
                .filter(String::isNotBlank).distinct(),
            providerNodes,
        )
    }

    suspend fun saveGroup(document: PolicyGroupDocument, draft: PolicyGroupDraft) {
        config.apply("singbox/outbounds", document.updated(draft), document.revision)
    }

    suspend fun addToGroups(member: PolicyMember, groups: Set<String>): Int {
        val snapshot = config.readSnapshot("singbox/outbounds")
        val outbounds = parsePolicySection(snapshot.content, "outbounds")
        var count = 0
        val updated = outbounds.map { original ->
            val draft = PolicyGroupDraft.from(original)
            if (draft.tag !in groups) return@map original
            require(draft.type in editablePolicyTypes)
            val next = draft.addPolicyMember(member)
            if (next == draft) return@map original
            count++
            val fields = original.toMutableMap()
            if (next.outbounds != draft.outbounds) fields["outbounds"] = JsonArray(next.outbounds.map(::JsonPrimitive))
            if (next.providers != draft.providers) fields["providers"] = JsonArray(next.providers.map(::JsonPrimitive))
            if (next.include != draft.include) fields["include"] = JsonPrimitive(next.include)
            JsonObject(fields)
        }
        require(outbounds.count { it.text("tag") in groups } == groups.size)
        if (count > 0) writeOutbounds(updated, snapshot.revision)
        return count
    }

    suspend fun testNodeLatency(identity: Long, member: PolicyMember): Int =
        client.testNodeLatency(identity, member.tag, member.providerTag)

    suspend fun createSubscriptionGroups(suffix: String): Int {
        val resources = catalogGroups()
        val providers = resources.filter { it.type == "subscription" && it.nodeCount > 0 }
            .map { it.runtimeTag }.filter(String::isNotBlank).distinct()
        if (providers.isEmpty()) return 0

        val document = groupDocument(null, resources)
        val dedicatedProviders = document.outbounds.map(PolicyGroupDraft::from).mapNotNull {
            if (it.type == "selector" && !it.useAll && it.outbounds.isEmpty()) it.providers.singleOrNull() else null
        }.toSet()
        val providerTags = (document.providerChoices + resources.map { it.runtimeTag }).filter(String::isNotBlank).toSet()
        val reserved = (document.memberTags + providerTags + "Compatible").toMutableSet()
        val additions = providers.filterNot(dedicatedProviders::contains).map { provider ->
            val base = "$provider ($suffix)"
            var tag = base
            var number = 2
            while (tag in reserved) tag = "$base (${number++})"
            reserved.add(tag)
            buildJsonObject {
                put("type", "selector")
                put("tag", tag)
                put("providers", JsonArray(listOf(JsonPrimitive(provider))))
            }
        }
        if (additions.isNotEmpty()) writeOutbounds(document.outbounds + additions, document.revision)
        return additions.size
    }

    suspend fun deleteGroup(tag: String, type: String) {
        val snapshot = config.readSnapshot("singbox/outbounds")
        val outbounds = parsePolicySection(snapshot.content, "outbounds")
        val group = outbounds.singleOrNull { it.text("tag") == tag } ?: return
        require(group.text("type") == type && type in editablePolicyTypes)
        writeOutbounds(outbounds.filterNot { it === group }, snapshot.revision)
    }

    private suspend fun writeOutbounds(outbounds: List<JsonObject>, revision: String) {
        val content = buildJsonObject { put("outbounds", JsonArray(outbounds)) }
        config.apply("singbox/outbounds", content.toString(), revision)
    }
    suspend fun snapshot(): PolicySnapshot = client.read<PolicySnapshot>("/policy-groups", mapOf("source" to "config"))
        .also { check(!it.live && it.coreIdentity == 0L) }

    suspend fun runtimeSnapshot(): PolicySnapshot = client.read("/policy-groups")

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
