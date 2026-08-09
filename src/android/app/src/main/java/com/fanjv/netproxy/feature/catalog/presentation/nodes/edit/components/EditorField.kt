package com.fanjv.netproxy.feature.catalog.presentation.nodes.edit.components

import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.unit.dp
import top.yukonga.miuix.kmp.basic.TextField

/** 编辑页共享的单行文本字段样式。 */
@Composable
internal fun EditorField(
    value: String,
    label: String,
    onValueChange: (String) -> Unit,
    onImeDone: () -> Unit
) {
    TextField(
        value = value,
        onValueChange = onValueChange,
        label = label,
        singleLine = true,
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 12.dp)
            .padding(bottom = 12.dp),
        keyboardActions = KeyboardActions(onDone = { onImeDone() }),
        keyboardOptions = KeyboardOptions(imeAction = ImeAction.Done)
    )
}


