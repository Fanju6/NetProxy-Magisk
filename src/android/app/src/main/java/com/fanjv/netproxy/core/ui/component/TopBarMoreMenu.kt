package com.fanjv.netproxy.core.ui.component

import androidx.compose.runtime.Composable
import androidx.compose.runtime.Immutable
import androidx.compose.ui.graphics.vector.ImageVector
import top.yukonga.miuix.kmp.basic.DropdownEntry
import top.yukonga.miuix.kmp.basic.DropdownItem
import top.yukonga.miuix.kmp.basic.Icon

@Immutable
internal data class TopBarMenuAction(
    val text: String,
    val enabled: Boolean = true,
    val icon: ImageVector? = null,
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
        listOf(DropdownEntry(actions.map { action ->
            DropdownItem(text = action.text, enabled = action.enabled, onClick = action.onClick,
                icon = action.icon?.let { vector -> { modifier -> Icon(vector, null, modifier) } })
        })),
        contentDescription,
        page,
    )
}
