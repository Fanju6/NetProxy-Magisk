package com.fanjv.netproxy.core.ui.component

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import top.yukonga.miuix.kmp.basic.ButtonDefaults
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.basic.TextButton
import top.yukonga.miuix.kmp.theme.MiuixTheme.colorScheme

@Composable
internal fun EmptyCatalog(
    text: String,
    onRefresh: (() -> Unit)?,
    modifier: Modifier = Modifier
) {
    Column(
        modifier = modifier,
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(12.dp)
    ) {
        Text(
            text = text,
            color = colorScheme.onSurfaceVariantSummary,
            fontSize = 14.sp,
            fontWeight = FontWeight.Medium,
            maxLines = 4,
            overflow = TextOverflow.Ellipsis
        )
        if (onRefresh != null) {
            TextButton(
                text = "继续",
                colors = ButtonDefaults.textButtonColorsPrimary(),
                onClick = onRefresh
            )
        }
    }
}
