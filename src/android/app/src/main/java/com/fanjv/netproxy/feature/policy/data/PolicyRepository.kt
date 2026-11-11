package com.fanjv.netproxy.feature.policy.data

import com.fanjv.netproxy.core.api.ModuleApiClient
import com.fanjv.netproxy.feature.policy.model.PolicyModel
import com.fanjv.netproxy.feature.policy.model.PolicyResponse
import com.fanjv.netproxy.feature.policy.model.PolicySaveResponse
import com.fanjv.netproxy.feature.catalog.model.NodeDelayResult
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.decodeFromJsonElement
import kotlinx.serialization.json.put
import kotlinx.serialization.json.putJsonArray
import kotlinx.serialization.json.add
import kotlinx.coroutines.flow.map

internal class PolicyRepository(private val client: ModuleApiClient) {
    suspend fun list(): List<PolicyModel> = client.read<PolicyResponse>("/policies").policies


    fun observeGroups() = client.observe(setOf("groups")).map { result -> result.map { Unit } }

    suspend fun save(policy: PolicyModel) {
        client.put("/policies/${policy.id}", buildJsonObject {
            put("group_id", policy.groupId)
            put("node_tag", policy.nodeTag)
            put("mode", policy.mode)
            put("region", policy.region)
        })
    }

    suspend fun create(name: String, packages: List<String>, domains: List<String>, ports: List<Int>): PolicyModel =
        client.post("/policies", buildJsonObject {
            put("name", name)
            put("mode", "proxy")
            putJsonArray("package_names") { packages.forEach(::add) }
            putJsonArray("domain_suffixes") { domains.forEach(::add) }
            putJsonArray("ports") { ports.forEach(::add) }
        }).data.let { client.json.decodeFromJsonElement<PolicySaveResponse>(it).savedPolicy() }

    suspend fun updateDefinition(policy: PolicyModel, name: String, packages: List<String>, domains: List<String>, ports: List<Int>): PolicyModel =
        client.put("/policies/${policy.id}", buildJsonObject {
            put("name", name)
            putJsonArray("package_names") { packages.forEach(::add) }
            putJsonArray("domain_suffixes") { domains.forEach(::add) }
            putJsonArray("ports") { ports.forEach(::add) }
            put("group_id", policy.groupId)
            put("node_tag", policy.nodeTag)
            put("mode", policy.mode)
            put("region", policy.region)
        }).data.let { client.json.decodeFromJsonElement<PolicySaveResponse>(it).savedPolicy() }

    suspend fun delete(id: String) {
        client.delete("/policies/$id")
    }

    suspend fun setDisabled(id: String, disabled: Boolean) {
        client.put("/policies/$id", buildJsonObject { put("disabled", disabled) })
    }

    suspend fun test(policy: PolicyModel): NodeDelayResult = client.json.decodeFromJsonElement<NodeDelayResult>(
        client.post("/policies/${policy.id}/url-test").data
    )

}
