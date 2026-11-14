package com.fanjv.netproxy.navigation

import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import com.fanjv.netproxy.core.ui.component.ManagerNavigationBar
import top.yukonga.miuix.kmp.basic.NavigationItem
import top.yukonga.miuix.kmp.blur.LayerBackdrop

/** 主界面唯一的 Miuix 底部导航栏。 */
@Composable
internal fun MainBottomBar(
    mainState: MainPagerState,
    blurBackdrop: LayerBackdrop?,
    items: List<NavigationItem>,
    modifier: Modifier = Modifier,
) {
    ManagerNavigationBar(items, mainState.selectedPage, { mainState.animateToPage(it) }, blurBackdrop, modifier)
}
