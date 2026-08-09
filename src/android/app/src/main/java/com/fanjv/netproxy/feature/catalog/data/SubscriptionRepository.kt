package com.fanjv.netproxy.feature.catalog.data

import com.fanjv.netproxy.feature.catalog.model.CatalogGroupSummary
import com.fanjv.netproxy.feature.catalog.model.CatalogNodeGroup
import com.fanjv.netproxy.feature.catalog.model.SubscriptionDraft
import com.fanjv.netproxy.feature.catalog.model.SubscriptionEditorState
import com.fanjv.netproxy.feature.catalog.model.SubscriptionHistoryEntry
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import java.io.File

/** 订阅元数据、更新事务与历史记录的数据入口。 */
internal class SubscriptionRepository(
    private val catalog: CatalogRepository
) {
    suspend fun list(): List<CatalogGroupSummary> =
        catalog.decode("catalog", "list")

    suspend fun details(id: String): CatalogNodeGroup =
        catalog.decode("catalog", "show", id)

    suspend fun readEditor(id: String): SubscriptionEditorState =
        catalog.decode("sub", "show", "--private", id)

    suspend fun add(draft: SubscriptionDraft): String =
        withHeadersFile(draft.customHeaders) { headersFile ->
            // 名称为空时整个省略该位置参数，由模块自动取名；传空串会被当作显式空名称
            val args = mutableListOf("sub", "add")
            appendOptions(args, draft, headersFile, addMode = true)
            if (draft.name.isNotBlank()) args.add(draft.name)
            args.add(draft.url)
            catalog.execute(*args.toTypedArray()).data.jsonObject["group_id"]
                ?.jsonPrimitive?.content.orEmpty()
        }

    suspend fun edit(
        id: String,
        original: SubscriptionEditorState,
        updated: SubscriptionDraft
    ) {
        val originalHeaders = original.customHeaders.mapValues { (_, value) ->
            value.jsonPrimitive.content
        }
        val headersChanged = originalHeaders != updated.customHeaders
        withHeadersFile(updated.customHeaders, required = headersChanged) { headersFile ->
            val args = mutableListOf("sub", "edit")
            if (original.name != updated.name) args += listOf("--name", updated.name)
            if (original.url != updated.url) args += listOf("--url", updated.url)
            if (original.userAgent != updated.userAgent) {
                args += listOf("--user-agent", updated.userAgent)
            }
            if (original.hwid != updated.hwid) args += listOf("--hwid", updated.hwid)
            if (headersChanged && headersFile != null) {
                args += listOf("--headers-file", headersFile.absolutePath)
            }
            if (original.updateInterval != updated.updateIntervalSeconds) {
                args += listOf("--interval", updated.updateIntervalSeconds.toString())
            }
            if (original.updateViaProxy != updated.updateViaProxy) {
                args += listOf("--via-proxy", updated.updateViaProxy)
            }
            if (original.include != updated.include) args += listOf("--include", updated.include)
            if (original.exclude != updated.exclude) args += listOf("--exclude", updated.exclude)
            if (original.allowInsecure != updated.allowInsecure) {
                args += "--allow-insecure=${updated.allowInsecure}"
            }
            if (original.timeout != updated.timeoutSeconds) {
                args += listOf("--download-timeout", updated.timeoutSeconds.toString())
            }
            if (original.autoUpdate != updated.autoUpdate) {
                args += "--auto-update=${updated.autoUpdate}"
            }
            args += id
            if (args.size > 3) catalog.execute(*args.toTypedArray())
        }
    }

    suspend fun update(id: String) {
        catalog.execute("sub", "update", id)
    }

    suspend fun updateAll() {
        catalog.execute("sub", "update-all")
    }

    suspend fun activate(id: String) {
        catalog.execute("node", "use", "auto", id)
    }

    suspend fun remove(id: String, replacementGroupId: String = "") {
        val args = mutableListOf("sub", "remove", id)
        replacementGroupId.takeIf(String::isNotBlank)?.let(args::add)
        catalog.execute(*args.toTypedArray())
    }

    suspend fun cancelUpdate(id: String) {
        catalog.execute("sub", "cancel", id)
    }

    suspend fun history(id: String): List<SubscriptionHistoryEntry> =
        catalog.decode("sub", "history", id)

    private fun appendOptions(
        args: MutableList<String>,
        draft: SubscriptionDraft,
        headersFile: File?,
        addMode: Boolean
    ) {
        if (draft.userAgent.isNotBlank()) args += listOf("--user-agent", draft.userAgent)
        if (draft.hwid.isNotBlank()) args += listOf("--hwid", draft.hwid)
        if (headersFile != null) args += listOf("--headers-file", headersFile.absolutePath)
        args += listOf("--interval", draft.updateIntervalSeconds.toString())
        args += listOf("--via-proxy", draft.updateViaProxy)
        if (draft.include.isNotBlank()) args += listOf("--include", draft.include)
        if (draft.exclude.isNotBlank()) args += listOf("--exclude", draft.exclude)
        if (draft.allowInsecure) {
            args += "--allow-insecure"
        } else if (!addMode) {
            args += "--allow-insecure=false"
        }
        args += listOf("--download-timeout", draft.timeoutSeconds.toString())
        args += "--auto-update=${draft.autoUpdate}"
    }

    private suspend fun <T> withHeadersFile(
        headers: Map<String, String>,
        required: Boolean = headers.isNotEmpty(),
        block: suspend (File?) -> T
    ): T {
        if (!required) return block(null)
        val content = catalog.json.encodeToString(
            JsonObject(headers.mapValues { JsonPrimitive(it.value) })
        )
        return catalog.withTextFile("netproxy-headers-", ".json", content, block)
    }
}
