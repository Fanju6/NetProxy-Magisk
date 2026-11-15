package com.fanjv.netproxy.core.ui.component

import androidx.compose.runtime.Composable
import androidx.compose.runtime.Immutable
import top.yukonga.miuix.kmp.basic.DropdownEntry
import top.yukonga.miuix.kmp.basic.DropdownItem

@Immutable
internal data class TopBarMenuAction(
    val text: String,
    val enabled: Boolean = true,
    val onClick: () -> Unit
)

/** 统一二级页面右上角的更多操作菜单。 */
@Composable
internal fun TopBarMoreMenu(
    actions: List<TopBarMenuAction>,
    contentDescription: String,
    page: @Composable (@Composable () -> Unit) -> Unit,
) {
    ManagerMenu(
        listOf(DropdownEntry(actions.map { DropdownItem(text = it.text, enabled = it.enabled, onClick = it.onClick) })),
        contentDescription,
        page,
    )
}
