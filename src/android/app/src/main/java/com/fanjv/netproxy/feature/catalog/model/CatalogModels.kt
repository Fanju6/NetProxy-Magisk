package com.fanjv.netproxy.feature.catalog.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

@Serializable
internal data class CatalogGroupSummary(
    val id: String,
    val name: String,
    val type: String,
    val active: Boolean = false,
    @SerialName("node_count") val nodeCount: Int = 0,
    val revision: Long = 0,
    @SerialName("auto_update") val autoUpdate: Boolean = false,
    @SerialName("update_interval") val updateInterval: Long = 86400,
    @SerialName("update_via_proxy") val updateViaProxy: String = "auto",
    val usage: SubscriptionUsage? = null,
    @SerialName("profile_title") val profileTitle: String = "",
    @SerialName("profile_web_page_url") val profileWebPageUrl: String = "",
    @SerialName("last_attempt_at") val lastAttemptAt: String = "",
    @SerialName("last_success_at") val lastSuccessAt: String = "",
    @SerialName("next_update_at") val nextUpdateAt: String = "",
    @SerialName("last_error") val lastError: String = "",
    @SerialName("updated_at") val updatedAt: String = "",
    val progress: SubscriptionProgress? = null
)

@Serializable
internal data class SubscriptionUsage(
    val upload: Long? = null,
    val download: Long? = null,
    val total: Long? = null,
    val expire: Long? = null
)

@Serializable
internal data class SubscriptionProgress(
    @SerialName("group_id") val groupId: String = "",
    val stage: String = "",
    val message: String = "",
    @SerialName("updated_at") val updatedAt: String = ""
)

@Serializable
internal data class CatalogNode(
    val tag: String,
    val protocol: String = "",
    val server: String = "",
    val port: Int = 0
)

@Serializable
internal data class CatalogNodeGroup(
    val group: CatalogGroupSummary,
    val nodes: List<CatalogNode> = emptyList()
)

@Serializable
internal data class CurrentNodeSelection(
    @SerialName("active_group_id") val activeGroupId: String = "",
    @SerialName("selector_mode") val selectorMode: String = "urltest",
    val selected: String = ""
)

@Serializable
internal data class CatalogNodesSnapshot(
    val groups: List<CatalogNodeGroup> = emptyList(),
    val selection: CurrentNodeSelection = CurrentNodeSelection()
)

@Serializable
internal data class NodeDelayResult(
    val target: String = "",
    val groups: List<ServiceOutboundGroup> = emptyList()
)

@Serializable
internal data class ServiceOutboundGroup(
    val tag: String = "",
    val items: List<ServiceOutboundItem> = emptyList()
)

@Serializable
internal data class ServiceOutboundItem(
    val tag: String = "",
    @SerialName("url_test_time") val urlTestTime: Long? = null,
    @SerialName("url_test_delay") val urlTestDelay: Int? = null
)
