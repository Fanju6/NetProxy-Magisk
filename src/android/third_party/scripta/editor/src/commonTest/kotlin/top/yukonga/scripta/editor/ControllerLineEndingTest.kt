package top.yukonga.scripta.editor

import androidx.compose.runtime.saveable.SaverScope
import kotlin.test.Test
import kotlin.test.assertEquals

/** CRLF 保真（C 组）：换行风格检测、getText(lineEnding) 还原、跨换文档/恢复的携带。 */
class ControllerLineEndingTest {

    @Test
    fun lfDocumentDetectsLF() {
        assertEquals(LineEnding.LF, CodeEditorController("a\nb\nc").lineEnding)
    }

    @Test
    fun crlfDocumentDetectsCRLF() {
        assertEquals(LineEnding.CRLF, CodeEditorController("a\r\nb\r\nc").lineEnding)
    }

    @Test
    fun mixedLineEndingsMajorityWins() {
        assertEquals(LineEnding.CRLF, CodeEditorController("a\r\nb\r\nc\nd").lineEnding)
        assertEquals(LineEnding.LF, CodeEditorController("a\r\nb\nc\nd").lineEnding)
    }

    @Test
    fun tieAndNoNewlineFallToLF() {
        assertEquals(LineEnding.LF, CodeEditorController("abc").lineEnding)
        assertEquals(LineEnding.LF, CodeEditorController("a\r\nb\nc").lineEnding) // 1:1 平手
        assertEquals(LineEnding.LF, CodeEditorController("").lineEnding)
    }

    @Test
    fun loneCarriageReturnsCountAsLF() {
        assertEquals(LineEnding.LF, CodeEditorController("a\rb\rc").lineEnding)
    }

    @Test
    fun setDocumentRedetectsLineEnding() {
        val c = CodeEditorController("a\nb")
        assertEquals(LineEnding.LF, c.lineEnding)
        c.setDocument("x\r\ny")
        assertEquals(LineEnding.CRLF, c.lineEnding)
    }

    @Test
    fun getTextWithLineEndingRestoresOriginalStyle() {
        val c = CodeEditorController("a\r\nb\r\nc")
        assertEquals("a\nb\nc", c.getText()) // 无参恒 LF 规范形
        assertEquals("a\r\nb\r\nc", c.getText(c.lineEnding)) // 保存路径：按原风格还原
    }

    @Test
    fun getTextWithExplicitStyleConverts() {
        val c = CodeEditorController("a\nb")
        assertEquals("a\r\nb", c.getText(LineEnding.CRLF)) // 强制换风格导出
        assertEquals("a\nb", c.getText(LineEnding.LF))
    }

    @Test
    fun saverCarriesLineEndingAcrossRestore() {
        val c = CodeEditorController("a\r\nb")
        val saved = with(CodeEditorController.Saver) { SaverScope { true }.save(c) }!!
        val r = CodeEditorController.Saver.restore(saved)!!
        // 恢复后的文本是 LF 规范形（保存的就是 getText()），风格必须随行囊携带、不能靠重检测。
        assertEquals(LineEnding.CRLF, r.lineEnding)
        assertEquals("a\r\nb", r.getText(r.lineEnding))
    }
}
