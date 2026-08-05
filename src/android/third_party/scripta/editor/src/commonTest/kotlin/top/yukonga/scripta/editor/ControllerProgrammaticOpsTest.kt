package top.yukonga.scripta.editor

import top.yukonga.scripta.editor.text.TextPosition
import top.yukonga.scripta.editor.text.TextRange
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/** Controller 编程操作（B3+B4）：select/jumpTo/jumpToLine、revealTick、代理对钳制、insertText/replaceRange。 */
class ControllerProgrammaticOpsTest {

    private fun controller(text: String): CodeEditorController = CodeEditorController(text)

    // --- B3 select / jumpTo ---

    @Test
    fun selectSetsDirectionalSelection() {
        val c = controller("hello\nworld")
        c.select(TextPosition(0, 1), TextPosition(1, 2))
        assertEquals(TextRange(TextPosition(0, 1), TextPosition(1, 2)), c.selection)
        assertEquals(TextPosition(1, 2), c.caret) // caret = end 参数（活动端）
    }

    @Test
    fun selectReversedPutsCaretAtStartOfRange() {
        val c = controller("hello\nworld")
        c.select(TextPosition(1, 2), TextPosition(0, 1))
        assertEquals(TextRange(TextPosition(0, 1), TextPosition(1, 2)), c.selection)
        assertEquals(TextPosition(0, 1), c.caret)
    }

    @Test
    fun jumpToMovesCaretAndClearsSelection() {
        val c = controller("a\nb\nc")
        c.engine.setSelection(TextPosition(0, 0), TextPosition(1, 1))
        c.jumpTo(TextPosition(2, 1))
        assertEquals(TextPosition(2, 1), c.caret)
        assertTrue(c.selection.isEmpty)
    }

    @Test
    fun jumpToClampsOutOfRange() {
        val c = controller("ab\ncd")
        c.jumpTo(TextPosition(99, 99))
        assertEquals(TextPosition(1, 2), c.caret)
    }

    @Test
    fun jumpToLineGoesToLineStart() {
        val c = controller("a\nbb\nccc")
        c.jumpToLine(2)
        assertEquals(TextPosition(2, 0), c.caret)
    }

    @Test
    fun programmaticNavigationBumpsRevealTick() {
        // 同位跳转也必须触发露出：selection 与旧值相等时快照不失效，revealTick 是强制通道。
        val c = controller("a\nb")
        c.jumpTo(TextPosition(0, 0)) // 光标本就在 (0,0)——正是「重复点同一 lint 错误」场景
        val t1 = c.engine.revealTick
        c.jumpTo(TextPosition(0, 0))
        assertTrue(c.engine.revealTick > t1)
    }

    @Test
    fun gotoLineJumpBumpsRevealTick() {
        val c = controller("a\nb\nc")
        val goto = c.gotoLine
        goto.open()
        goto.input = "1" // 跳到当前行（同位）
        val t0 = c.engine.revealTick
        goto.jump()
        assertTrue(c.engine.revealTick > t0)
    }

    @Test
    fun surrogatePairColumnSnapsToCodePointBoundary() {
        // "a😀b"：😀 占 col1(高位)+col2(低位)。跳到 col2（代理对中间）须回退到 col1。
        val c = controller("a😀b")
        c.jumpTo(TextPosition(0, 2))
        assertEquals(TextPosition(0, 1), c.caret)
    }

    // --- B4 insertText / replaceRange ---

    @Test
    fun insertTextInsertsAtCaretAndUndoesInOneStep() {
        val c = controller("ab")
        c.engine.setCursor(TextPosition(0, 1))
        c.insertText("X")
        assertEquals("aXb", c.getText())
        assertEquals(TextPosition(0, 2), c.caret)
        c.undo()
        assertEquals("ab", c.getText())
    }

    @Test
    fun insertTextReplacesSelection() {
        val c = controller("hello")
        c.engine.setSelection(TextPosition(0, 1), TextPosition(0, 4))
        c.insertText("X")
        assertEquals("hXo", c.getText())
    }

    @Test
    fun insertTextPreservesActivePreedit() {
        // 镜像 pinyinComposeThenCommitToHanzi：预编辑必须先定格上屏，宿主文本不得顶替它。
        val c = controller("")
        c.engine.setComposingText("nihao", 1)
        c.insertText("X")
        assertEquals("nihaoX", c.getText())
        assertEquals(null, c.engine.composing)
    }

    @Test
    fun replaceRangePreservesActivePreedit() {
        val c = controller("ab")
        c.engine.setCursor(TextPosition(0, 2))
        c.engine.setComposingText("ni", 1)
        c.replaceRange(TextPosition(0, 0), TextPosition(0, 1), "Z")
        assertEquals("Zbni", c.getText()) // 预编辑 "ni" 定格保留，a 被替换为 Z
        assertEquals(null, c.engine.composing)
    }

    @Test
    fun replaceRangeReplacesAndCollapsesToEnd() {
        val c = controller("hello world")
        c.replaceRange(TextPosition(0, 0), TextPosition(0, 5), "bye")
        assertEquals("bye world", c.getText())
        assertEquals(TextPosition(0, 3), c.caret)
        c.undo()
        assertEquals("hello world", c.getText())
    }

    @Test
    fun replaceRangeClampsEndpoints() {
        val c = controller("ab")
        c.replaceRange(TextPosition(0, 1), TextPosition(9, 9), "X")
        assertEquals("aX", c.getText())
    }

    @Test
    fun insertTextDoesNotMergeWithPrecedingUserTyping() {
        // 编程插入是语义断点：不并入用户键入单元，否则一次 undo 连用户自己的词一起吞掉。
        val c = controller("")
        c.engine.insert("h"); c.engine.insert("i") // 用户连续键入（合并为一单元）
        c.insertText("!")
        c.undo()
        assertEquals("hi", c.getText())
    }

    @Test
    fun userTypingAfterInsertTextDoesNotMergeIn() {
        val c = controller("")
        c.insertText("!")
        c.engine.insert("a")
        c.undo()
        assertEquals("!", c.getText())
    }

    @Test
    fun emptyInsertTextWithEmptySelectionIsNoOp() {
        // 空插入不产生编辑：不置脏、不留空撤销单元。
        val c = controller("ab")
        c.insertText("")
        assertEquals(false, c.isModified)
        assertEquals(false, c.canUndo)
    }

    @Test
    fun emptyReplaceOfEmptyRangeIsNoOp() {
        val c = controller("ab")
        c.replaceRange(TextPosition(0, 1), TextPosition(0, 1), "")
        assertEquals(false, c.isModified)
        assertEquals(false, c.canUndo)
    }

    @Test
    fun replaceRangeBumpsRevealTick() {
        val c = controller("hello")
        val t0 = c.engine.revealTick
        c.replaceRange(TextPosition(0, 0), TextPosition(0, 5), "hi")
        assertTrue(c.engine.revealTick > t0)
    }

    @Test
    fun replaceRangeWholeDocumentKeepsUndoHistory() {
        val c = controller("old\ncontent")
        c.engine.insert("!") // 先制造一条历史
        c.replaceRange(TextPosition(0, 0), TextPosition(1, 7), "new")
        assertEquals("new", c.getText())
        assertTrue(c.canUndo)
        c.undo()
        assertEquals("!old\ncontent", c.getText()) // 全文替换只回退一步，此前历史仍在
        assertTrue(c.canUndo)
    }
}
