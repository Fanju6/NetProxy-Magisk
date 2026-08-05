package top.yukonga.scripta.editor.input

import android.content.ClipData
import androidx.compose.ui.platform.ClipEntry

internal actual fun plainTextClipEntry(text: String): ClipEntry =
    ClipEntry(ClipData.newPlainText(null, text))

internal actual fun ClipEntry.plainText(): String? {
    if (clipData.itemCount == 0) return null
    return clipData.getItemAt(0).text?.toString()
}
