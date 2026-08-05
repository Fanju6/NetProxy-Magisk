package com.fanjv.netproxy.navigation

import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import com.fanjv.netproxy.core.ui.component.BlurredBar
import com.fanjv.netproxy.core.ui.component.bottombar.IosLiquidGlassNavigationBar
import top.yukonga.miuix.kmp.basic.NavigationBar
import top.yukonga.miuix.kmp.basic.NavigationBarItem
import top.yukonga.miuix.kmp.basic.NavigationItem
import top.yukonga.miuix.kmp.blur.LayerBackdrop
import top.yukonga.miuix.kmp.theme.MiuixTheme

/** 主界面底栏：按设置切换浮动液态玻璃导航栏或标准 Miuix 导航栏。 */
@Composable
internal fun MainBottomBar(
    mainState: MainPagerState,
    blurBackdrop: LayerBackdrop?,
    items: List<NavigationItem>,
    enableFloatingBottomBar: Boolean,
    enableFloatingBottomBarBlur: Boolean,
    modifier: Modifier = Modifier,
) {
    if (enableFloatingBottomBar) {
        val blurActive = enableFloatingBottomBarBlur && blurBackdrop != null
        IosLiquidGlassNavigationBar(
            items = items,
            selectedIndex = mainState.selectedPage,
            onItemClick = { mainState.animateToPage(it) },
            backdrop = blurBackdrop,
            isBlurActive = blurActive,
            modifier = modifier,
        )
    } else {
        val blurActive = blurBackdrop != null
        BlurredBar(backdrop = blurBackdrop) {
            NavigationBar(
                modifier = modifier,
                color = if (blurActive) Color.Transparent else MiuixTheme.colorScheme.surface,
                content = {
                    items.forEachIndexed { index, item ->
                        NavigationBarItem(
                            modifier = Modifier.weight(1f),
                            icon = item.icon,
                            label = item.label,
                            selected = mainState.selectedPage == index,
                            onClick = {
                                mainState.animateToPage(index)
                            }
                        )
                    }
                }
            )
        }
    }
}
