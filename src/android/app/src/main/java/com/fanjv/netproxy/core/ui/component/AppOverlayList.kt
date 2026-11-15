package com.fanjv.netproxy.core.ui.component

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.padding
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import top.yukonga.miuix.kmp.basic.HorizontalDivider
import top.yukonga.miuix.kmp.theme.MiuixTheme.colorScheme

@Composable
internal fun AppOverlayList(modifier: Modifier = Modifier, content: @Composable ColumnScope.() -> Unit) {
    Column(modifier, content = content)
}

@Composable
internal fun AppOverlayListDivider() {
    HorizontalDivider(
        modifier = Modifier.padding(horizontal = 16.dp),
        thickness = Dp.Hairline,
        color = colorScheme.onSurface.copy(alpha = 0.12f),
    )
}
