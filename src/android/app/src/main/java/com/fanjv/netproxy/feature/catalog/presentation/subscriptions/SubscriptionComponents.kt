package com.fanjv.netproxy.feature.catalog.presentation.subscriptions

import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.ui.component.AppBottomSheet as OverlayBottomSheet
import com.fanjv.netproxy.core.ui.component.chromeIcon
import com.fanjv.netproxy.feature.catalog.model.CatalogGroupSummary
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import top.yukonga.miuix.kmp.basic.Icon
import top.yukonga.miuix.kmp.basic.InfiniteProgressIndicator
import top.yukonga.miuix.kmp.basic.SmallTitle
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.theme.MiuixTheme.colorScheme

@Composable
internal fun OverlayActionIcon(imageVector: androidx.compose.ui.graphics.vector.ImageVector, destructive: Boolean = false) {
    Icon(
        imageVector = chromeIcon(imageVector),
        modifier = Modifier.size(22.dp),
        tint = if (destructive) colorScheme.error else colorScheme.onSurfaceVariantSummary,
        contentDescription = null
    )
}

/** 新增/编辑 URL 订阅。敏感字段只通过 root CLI 的私有契约读取。 */

@Composable
internal fun SectionLabel(text: String) {
    SmallTitle(
        text = text,
        insideMargin = PaddingValues(start = 16.dp, top = 8.dp, end = 16.dp, bottom = 0.dp)
    )
}

@Composable
internal fun SubscriptionUpdateSheet(operation: String) {
    val updateAll = operation == "update-all"
    OverlayBottomSheet(
        show = operation == "update" || updateAll,
        title = stringResource(
            if (updateAll) R.string.subscription_update_all else R.string.common_update
        ),
        onDismissRequest = {}
    ) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(bottom = 12.dp),
            verticalAlignment = Alignment.CenterVertically
        ) {
            InfiniteProgressIndicator(modifier = Modifier.size(28.dp))
            Spacer(Modifier.width(16.dp))
            Text(
                text = if (updateAll) {
                    stringResource(R.string.subscription_update_sheet_all)
                } else {
                    stringResource(R.string.subscription_update_sheet_single)
                },
                color = colorScheme.onSurface
            )
        }
    }
}

internal enum class SubscriptionUpdateStatus(val label: Int) {
    Updating(R.string.subscription_update_in_progress),
    Failed(R.string.subscription_update_failed),
    Expired(R.string.subscription_expired),
    NeverUpdated(R.string.subscription_never_updated),
}

private val activeProgressStages = setOf("download", "convert", "validate", "apply")

internal fun subscriptionBadge(
    subscription: CatalogGroupSummary,
    now: Long = Instant.now().epochSecond,
): SubscriptionUpdateStatus? {
    if (subscription.type != "subscription") return null
    val expire = subscription.usage?.expire ?: 0L
    return when {
        subscription.progress?.stage?.let(activeProgressStages::contains) == true -> SubscriptionUpdateStatus.Updating
        subscription.lastError.isNotBlank() -> SubscriptionUpdateStatus.Failed
        expire > 0 && expire <= now -> SubscriptionUpdateStatus.Expired
        subscription.lastSuccessAt.isBlank() -> SubscriptionUpdateStatus.NeverUpdated
        else -> null
    }
}

internal fun formatDate(value: String): String = runCatching {
    DateTimeFormatter.ofPattern("MM-dd HH:mm")
        .withZone(ZoneId.systemDefault())
        .format(Instant.parse(value))
}.getOrDefault(value.ifBlank { "--" })
