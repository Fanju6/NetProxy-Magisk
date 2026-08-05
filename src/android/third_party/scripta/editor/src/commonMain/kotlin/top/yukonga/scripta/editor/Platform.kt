package top.yukonga.scripta.editor

import androidx.compose.ui.text.PlatformTextStyle

/**
 * Android: 返回 `PlatformTextStyle(includeFontPadding = false)`，关闭那份会给 CJK 回退字体额外
 * 行内上边距、把整行基线压低（导致同一行里英文相对中文向下偏移）的旧行为。桌面无此项，返回 null。
 */
internal expect fun editorNoFontPaddingStyle(): PlatformTextStyle?
