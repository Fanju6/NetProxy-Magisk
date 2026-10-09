package com.fanjv.netproxy.core.ui.component

import androidx.annotation.StringRes
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.runtime.Composable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.runtime.saveable.rememberSaveableStateHolder
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import com.fanjv.netproxy.core.module.ModuleAvailability
import top.yukonga.miuix.kmp.basic.InfiniteProgressIndicator
import top.yukonga.miuix.kmp.basic.Scaffold
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.blur.layerBackdrop
import top.yukonga.miuix.kmp.theme.MiuixTheme

internal val LocalModuleAvailability = staticCompositionLocalOf<ModuleAvailability?> { null }

@Composable
internal fun ContentStatus(
    padding: PaddingValues,
    text: String,
    loading: Boolean = false,
    modifier: Modifier = Modifier,
) {
    Box(modifier.fillMaxSize().padding(padding).padding(horizontal = 24.dp),
        contentAlignment = Alignment.Center) {
        if (loading) InfiniteProgressIndicator()
        else Text(text, style = MiuixTheme.textStyles.body2,
            color = MiuixTheme.colorScheme.onSurfaceVariantSummary, textAlign = TextAlign.Center)
    }
}

@Composable
internal fun ModulePage(
    @StringRes title: Int,
    @StringRes unavailable: Int,
    onBack: () -> Unit,
    content: @Composable () -> Unit,
) {
    val availability = LocalModuleAvailability.current
    // 暂时失去访问权限时仍保留编辑器的文本、revision 和导航条目的 ViewModel。
    val holder = rememberSaveableStateHolder()
    if (availability?.available == true) {
        holder.SaveableStateProvider("content", content)
    } else {
        val backdrop = rememberBlurBackdrop()
        Scaffold(topBar = {
            BlurredBar(backdrop) {
                AdaptiveTopAppBar(title = stringResource(title),
                    color = if (backdrop != null) Color.Transparent else MiuixTheme.colorScheme.surface,
                    navigationIcon = { BackIconButton(onClick = onBack) })
            }
        }) { padding ->
            ContentStatus(padding, stringResource(unavailable), loading = availability == null,
                modifier = if (backdrop != null) Modifier.layerBackdrop(backdrop) else Modifier)
        }
    }
}
