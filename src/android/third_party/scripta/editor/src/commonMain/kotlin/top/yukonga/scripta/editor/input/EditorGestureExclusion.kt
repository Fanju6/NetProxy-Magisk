package top.yukonga.scripta.editor.input

import androidx.compose.ui.Modifier

/**
 * 右缘滚动条热区的系统手势排除：Android 手势导航的返回手势区（右缘 ~20-32dp）与可抓 thumb 热区
 * 几乎重合，不排除则抓 thumb 起手稍带向左分量就被系统接管成返回、直接退出编辑屏。只在滚动条
 * 可抓期挂载（常驻排除会削弱整条右缘的返回手势）。非 Android 平台无此概念，恒等返回。
 */
expect fun Modifier.editorRightEdgeGestureExclusion(widthPx: Float): Modifier
