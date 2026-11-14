package com.fanjv.netproxy.core.ui.component

import androidx.compose.runtime.Composable
import androidx.compose.runtime.Immutable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import top.yukonga.miuix.kmp.basic.DropdownEntry
import top.yukonga.miuix.kmp.basic.DropdownItem
import top.yukonga.miuix.kmp.basic.DropdownImpl
import top.yukonga.miuix.kmp.basic.Icon
import top.yukonga.miuix.kmp.basic.IconButton
import top.yukonga.miuix.kmp.basic.ListPopupColumn
import top.yukonga.miuix.kmp.basic.ListPopupDefaults
import top.yukonga.miuix.kmp.basic.PopupPositionProvider
import top.yukonga.miuix.kmp.icon.MiuixIcons
import top.yukonga.miuix.kmp.icon.extended.MoreCircle
import top.yukonga.miuix.kmp.overlay.OverlayListPopup
import top.yukonga.miuix.kmp.theme.MiuixTheme.colorScheme

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
    if (useGlassChrome) {
        ManagerMenu(
            listOf(DropdownEntry(actions.map { DropdownItem(text = it.text, enabled = it.enabled, onClick = it.onClick) })),
            contentDescription,
            page,
        )
    } else page {
        var expanded by remember { mutableStateOf(false) }
        OverlayListPopup(
        show = expanded,
        popupPositionProvider = ListPopupDefaults.ContextMenuPositionProvider,
        alignment = PopupPositionProvider.Align.TopEnd,
        onDismissRequest = { expanded = false }
    ) {
        ListPopupColumn {
            actions.forEachIndexed { index, action ->
                DropdownImpl(
                    text = action.text,
                    optionSize = actions.size,
                    isSelected = false,
                    index = index,
                    enabled = action.enabled,
                    onSelectedIndexChange = {
                        expanded = false
                        action.onClick()
                    }
                )
            }
        }
    }
    IconButton(
        onClick = { expanded = true },
        holdDownState = expanded
    ) {
        Icon(
            imageVector = MiuixIcons.MoreCircle,
            contentDescription = contentDescription,
            tint = colorScheme.onSurface
        )
    }
    }
}
