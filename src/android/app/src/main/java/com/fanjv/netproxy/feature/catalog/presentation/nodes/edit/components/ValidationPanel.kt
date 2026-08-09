package com.fanjv.netproxy.feature.catalog.presentation.nodes.edit.components

import androidx.compose.runtime.Composable
import com.fanjv.netproxy.core.ui.component.SnackbarNoticeEffect
import top.yukonga.miuix.kmp.basic.SnackbarHostState

/** 编辑页校验提示的统一入口，仍使用应用现有 Snackbar 行为。 */
@Composable
internal fun ValidationPanel(
    eventId: Long,
    message: String,
    isError: Boolean,
    hostState: SnackbarHostState,
    onConsumed: () -> Unit
) {
    SnackbarNoticeEffect(
        eventId = eventId,
        message = message,
        isError = isError,
        hostState = hostState,
        onConsumed = onConsumed
    )
}


