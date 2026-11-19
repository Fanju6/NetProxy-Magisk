package top.yukonga.scripta.editor

import androidx.compose.runtime.compositionLocalOf
import androidx.compose.ui.unit.dp

/** Height covered by a floating host toolbar. The editor still draws behind it. */
val LocalEditorBottomContentInset = compositionLocalOf { 0.dp }
