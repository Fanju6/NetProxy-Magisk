package com.fanjv.netproxy.core.ui.component

import androidx.compose.foundation.layout.padding
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import top.yukonga.miuix.kmp.basic.SnackbarDuration
import top.yukonga.miuix.kmp.basic.SnackbarHost
import top.yukonga.miuix.kmp.basic.SnackbarHostState

@Composable
internal fun rememberAppSnackbarHostState(): SnackbarHostState =
    remember { SnackbarHostState() }

@Composable
internal fun AppSnackbarHost(
    state: SnackbarHostState,
    modifier: Modifier = Modifier
) {
    SnackbarHost(
        state = state,
        modifier = modifier.padding(horizontal = 12.dp)
    )
}

@Composable
internal fun SnackbarNoticeEffect(
    eventId: Long,
    message: String,
    isError: Boolean,
    hostState: SnackbarHostState,
    onConsumed: () -> Unit
) {
    LaunchedEffect(eventId) {
        if (message.isBlank()) return@LaunchedEffect
        onConsumed()
        hostState.showSnackbar(
            message = message,
            withDismissAction = isError,
            duration = if (isError) SnackbarDuration.Long else SnackbarDuration.Short
        )
    }
}
