package top.yukonga.scripta.editor.input

import androidx.compose.ui.platform.ClipEntry

/**
 * 纯文本 <-> 平台 [ClipEntry] 的桥。Compose 1.11 的新 [androidx.compose.ui.platform.Clipboard] 只收发
 * [ClipEntry]，而 foundation 里的文本转换扩展是 internal、模块外用不到——故编辑器自带这一小层。
 */
internal expect fun plainTextClipEntry(text: String): ClipEntry

/** 从 [ClipEntry] 取纯文本；不含文本时返回 null。 */
internal expect fun ClipEntry.plainText(): String?
