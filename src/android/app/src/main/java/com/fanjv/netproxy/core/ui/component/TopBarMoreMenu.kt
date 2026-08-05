package com.fanjv.netproxy.core.ui.component

import androidx.compose.runtime.Composable
import androidx.compose.runtime.Immutable
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
    expanded: Boolean,
    onExpandedChange: (Boolean) -> Unit,
    actions: List<TopBarMenuAction>,
    contentDescription: String
) {
    OverlayListPopup(
        show = expanded,
        popupPositionProvider = ListPopupDefaults.ContextMenuPositionProvider,
        alignment = PopupPositionProvider.Align.TopEnd,
        onDismissRequest = { onExpandedChange(false) }
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
                        onExpandedChange(false)
                        action.onClick()
                    }
                )
            }
        }
    }
    IconButton(
        onClick = { onExpandedChange(true) },
        holdDownState = expanded
    ) {
        Icon(
            imageVector = MiuixIcons.MoreCircle,
            contentDescription = contentDescription,
            tint = colorScheme.onSurface
        )
    }
}
