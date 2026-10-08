package com.fanjv.netproxy.feature.catalog.presentation.subscriptions

import com.fanjv.netproxy.core.command.NetProxyCtlException
import com.fanjv.netproxy.core.ui.UiText
import com.fanjv.netproxy.feature.catalog.model.SubscriptionDraft
import com.fanjv.netproxy.feature.catalog.model.SubscriptionEditorState
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonPrimitive

internal data class SubscriptionEditorUiState(
    val id: String = "",
    val original: SubscriptionEditorState? = null,
    val draft: SubscriptionDraft = SubscriptionDraft(name = "", url = ""),
    val headersText: String = "",
    val loading: Boolean = false,
    val saving: Boolean = false,
    val saved: Boolean = false,
    val persisted: Boolean = false,
    val baselineReadRequired: Boolean = false,
    val runtimeSyncState: String = "",
    val runtimeSyncPending: Boolean = false,
    val error: UiText = UiText.Empty,
    val noticeId: Long = 0
)

internal fun SubscriptionEditorState.toDraft() = SubscriptionDraft(
    name = name, url = url, userAgent = userAgent, hwid = hwid,
    customHeaders = customHeaders.mapValues { it.value.jsonPrimitive.content },
    autoUpdate = autoUpdate, updateIntervalSeconds = updateInterval,
    updateViaProxy = updateViaProxy, include = include, exclude = exclude,
    allowInsecure = allowInsecure, timeoutSeconds = timeout
)

internal fun SubscriptionEditorState.headersText(): String =
    customHeaders.entries.joinToString("\n") { (key, value) -> "$key: ${value.jsonPrimitive.content}" }

internal fun SubscriptionEditorUiState.sameInput(other: SubscriptionEditorUiState): Boolean =
    draft == other.draft && headersText == other.headersText

internal fun SubscriptionEditorUiState.recoverBaseline(editor: SubscriptionEditorState): SubscriptionEditorUiState = copy(
    id = editor.id,
    original = editor,
    baselineReadRequired = false,
    draft = if (original == null && draft.name.isBlank()) draft.copy(name = editor.name) else draft
)

// 回读推进磁盘基线和新增身份，但只校准提交后没有再修改的输入，不能覆盖用户的新草稿。
internal fun SubscriptionEditorUiState.rebase(
    submitted: SubscriptionEditorUiState,
    editor: SubscriptionEditorState
): SubscriptionEditorUiState {
    val disk = editor.toDraft()
    val before = submitted.draft
    fun <T> keep(current: T, previous: T, persisted: T): T = if (current == previous) persisted else current
    return copy(
        id = editor.id,
        original = editor,
        baselineReadRequired = false,
        loading = false,
        draft = draft.copy(
            name = keep(draft.name, before.name, disk.name),
            url = keep(draft.url, before.url, disk.url),
            userAgent = keep(draft.userAgent, before.userAgent, disk.userAgent),
            hwid = keep(draft.hwid, before.hwid, disk.hwid),
            customHeaders = keep(draft.customHeaders, before.customHeaders, disk.customHeaders),
            autoUpdate = keep(draft.autoUpdate, before.autoUpdate, disk.autoUpdate),
            updateIntervalSeconds = keep(draft.updateIntervalSeconds, before.updateIntervalSeconds, disk.updateIntervalSeconds),
            updateViaProxy = keep(draft.updateViaProxy, before.updateViaProxy, disk.updateViaProxy),
            include = keep(draft.include, before.include, disk.include),
            exclude = keep(draft.exclude, before.exclude, disk.exclude),
            allowInsecure = keep(draft.allowInsecure, before.allowInsecure, disk.allowInsecure),
            timeoutSeconds = keep(draft.timeoutSeconds, before.timeoutSeconds, disk.timeoutSeconds)
        ),
        headersText = keep(headersText, submitted.headersText, editor.headersText())
    )
}

internal data class SubscriptionRuntimeOutcome(
    val groupId: String = "",
    val persisted: Boolean = false,
    val state: String = "",
    val pending: Boolean = false
)

internal fun JsonElement?.subscriptionOutcome(): SubscriptionRuntimeOutcome {
    val data = this as? JsonObject
    return SubscriptionRuntimeOutcome(
        groupId = (data?.get("group_id") as? JsonPrimitive)?.contentOrNull.orEmpty(),
        persisted = (data?.get("persisted") as? JsonPrimitive)?.booleanOrNull == true,
        state = (data?.get("runtime_sync_state") as? JsonPrimitive)?.contentOrNull.orEmpty(),
        pending = (data?.get("runtime_sync_pending") as? JsonPrimitive)?.booleanOrNull == true
    )
}

internal fun Throwable.subscriptionPersisted(): Boolean =
    (this as? NetProxyCtlException)?.data.subscriptionOutcome().persisted

internal fun subscriptionIntervalOptions(current: Long): List<Long> {
    val presets = listOf(900L, 3600L, 21600L, 43200L, 86400L, 259200L, 604800L)
    return if (current in presets) presets else presets + current
}
