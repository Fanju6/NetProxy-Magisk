package com.fanjv.netproxy.feature.catalog.presentation.nodes.edit.components

import androidx.compose.runtime.Composable
import androidx.compose.ui.res.stringResource
import com.fanjv.netproxy.R
import top.yukonga.miuix.kmp.basic.Icon
import top.yukonga.miuix.kmp.basic.IconButton
import top.yukonga.miuix.kmp.icon.MiuixIcons
import top.yukonga.miuix.kmp.icon.extended.Ok
import top.yukonga.miuix.kmp.theme.MiuixTheme.colorScheme

/** 编辑页保存动作按钮。 */
@Composable
internal fun ActionButtons(onSave: () -> Unit, enabled: Boolean = true) {
    IconButton(onClick = onSave, enabled = enabled) {
        Icon(
            imageVector = MiuixIcons.Ok,
            contentDescription = stringResource(R.string.save_text),
            tint = colorScheme.onSurface
        )
    }
}



