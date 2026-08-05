@file:OptIn(ExperimentalComposeUiApi::class)

package top.yukonga.scripta.editor.input

import androidx.compose.ui.ExperimentalComposeUiApi
import androidx.compose.ui.platform.ClipEntry
import androidx.compose.ui.platform.asAwtTransferable
import java.awt.datatransfer.DataFlavor
import java.awt.datatransfer.StringSelection

internal actual fun plainTextClipEntry(text: String): ClipEntry =
    ClipEntry(StringSelection(text))

internal actual fun ClipEntry.plainText(): String? {
    val transferable = asAwtTransferable ?: return null
    if (!transferable.isDataFlavorSupported(DataFlavor.stringFlavor)) return null
    return transferable.getTransferData(DataFlavor.stringFlavor) as? String
}
