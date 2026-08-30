package com.fanjv.netproxy.navigation

import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import com.fanjv.netproxy.core.ui.component.BlurredBar
import top.yukonga.miuix.kmp.basic.NavigationBar
import top.yukonga.miuix.kmp.basic.NavigationBarItem
import top.yukonga.miuix.kmp.basic.NavigationItem
import top.yukonga.miuix.kmp.blur.LayerBackdrop
import top.yukonga.miuix.kmp.theme.MiuixTheme

/** 主界面唯一的 Miuix 底部导航栏。 */
@Composable
internal fun MainBottomBar(
    mainState: MainPagerState,
    blurBackdrop: LayerBackdrop?,
    items: List<NavigationItem>,
    modifier: Modifier = Modifier,
) {
    BlurredBar(backdrop = blurBackdrop) {
        NavigationBar(
            modifier = modifier.fillMaxWidth(),
            color = if (blurBackdrop != null) Color.Transparent else MiuixTheme.colorScheme.surface,
            content = {
                items.forEachIndexed { index, item ->
                    NavigationBarItem(
                        modifier = Modifier.weight(1f),
                        icon = item.icon,
                        label = item.label,
                        selected = mainState.selectedPage == index,
                        onClick = { mainState.animateToPage(index) },
                    )
                }
            },
        )
    }
}
