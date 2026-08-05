package top.yukonga.scripta.editor.text

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue

class UndoHistoryTest {

    private fun sel(o: Int) = SelectionState(o, o)

    @Test
    fun emptyHistoryCannotUndoOrRedo() {
        val h = UndoHistory()
        assertFalse(h.canUndo)
        assertFalse(h.canRedo)
        assertNull(h.undo())
        assertNull(h.redo())
    }

    @Test
    fun recordThenUndoReturnsInverseEditAndSelectionBefore() {
        val h = UndoHistory()
        h.record(TextEdit(3, "", "abc"), sel(3), sel(6), EditKind.Other)
        assertTrue(h.canUndo)
        val step = h.undo()
        assertNotNull(step)
        assertEquals(listOf(TextEdit(3, "abc", "")), step.edits)
        assertEquals(SelectionState(3, 3), step.selection)
        assertFalse(h.canUndo)
        assertTrue(h.canRedo)
    }

    @Test
    fun undoThenRedoRoundTrip() {
        val h = UndoHistory()
        h.record(TextEdit(0, "xy", "Z"), SelectionState(0, 2), sel(1), EditKind.Other)
        h.undo()
        val step = h.redo()
        assertNotNull(step)
        assertEquals(listOf(TextEdit(0, "xy", "Z")), step.edits)
        assertEquals(SelectionState(1, 1), step.selection)
        assertTrue(h.canUndo)
        assertFalse(h.canRedo)
    }

    @Test
    fun newRecordClearsRedoStack() {
        val h = UndoHistory()
        h.record(TextEdit(0, "", "a"), sel(0), sel(1), EditKind.Typing)
        h.undo()
        assertTrue(h.canRedo)
        h.record(TextEdit(0, "", "b"), sel(0), sel(1), EditKind.Typing)
        assertFalse(h.canRedo)
    }

    @Test
    fun typingMergesContiguousInserts() {
        val h = UndoHistory()
        h.record(TextEdit(0, "", "a"), sel(0), sel(1), EditKind.Typing)
        h.record(TextEdit(1, "", "b"), sel(1), sel(2), EditKind.Typing)
        h.record(TextEdit(2, "", "c"), sel(2), sel(3), EditKind.Typing)
        val step = h.undo()
        assertNotNull(step)
        assertEquals(listOf(TextEdit(0, "abc", "")), step.edits)
        assertEquals(SelectionState(0, 0), step.selection)
        assertFalse(h.canUndo)
    }

    @Test
    fun typingDoesNotMergeNonContiguousInserts() {
        val h = UndoHistory()
        h.record(TextEdit(0, "", "a"), sel(0), sel(1), EditKind.Typing)
        h.record(TextEdit(5, "", "b"), sel(5), sel(6), EditKind.Typing)
        assertNotNull(h.undo())
        assertTrue(h.canUndo) // 两个独立单元
    }

    @Test
    fun typingDoesNotMergeAcrossNewline() {
        val h = UndoHistory()
        h.record(TextEdit(0, "", "a"), sel(0), sel(1), EditKind.Typing)
        h.record(TextEdit(1, "", "\n"), sel(1), sel(2), EditKind.Typing)
        h.record(TextEdit(2, "", "b"), sel(2), sel(3), EditKind.Typing)
        // 换行自成单元，且其后的输入不并入换行单元 → 共 3 个单元。
        assertEquals(listOf(TextEdit(2, "b", "")), h.undo()?.edits)
        assertEquals(listOf(TextEdit(1, "\n", "")), h.undo()?.edits)
        assertEquals(listOf(TextEdit(0, "a", "")), h.undo()?.edits)
        assertFalse(h.canUndo)
    }

    @Test
    fun breakMergeSplitsTypingRun() {
        val h = UndoHistory()
        h.record(TextEdit(0, "", "a"), sel(0), sel(1), EditKind.Typing)
        h.breakMerge()
        h.record(TextEdit(1, "", "b"), sel(1), sel(2), EditKind.Typing)
        assertEquals(listOf(TextEdit(1, "b", "")), h.undo()?.edits)
        assertEquals(listOf(TextEdit(0, "a", "")), h.undo()?.edits)
    }

    @Test
    fun backspaceRunMergesBackwards() {
        // "hello" 从末尾连退 o、l、l。
        val h = UndoHistory()
        h.record(TextEdit(4, "o", ""), sel(5), sel(4), EditKind.DeleteBackward)
        h.record(TextEdit(3, "l", ""), sel(4), sel(3), EditKind.DeleteBackward)
        h.record(TextEdit(2, "l", ""), sel(3), sel(2), EditKind.DeleteBackward)
        val step = h.undo()
        assertEquals(listOf(TextEdit(2, "", "llo")), step?.edits)
        assertEquals(SelectionState(5, 5), step?.selection)
        assertFalse(h.canUndo)
    }

    @Test
    fun deleteForwardRunMergesAtSameOffset() {
        // "hello" 在 offset 2 连续前删 l、l、o。
        val h = UndoHistory()
        h.record(TextEdit(2, "l", ""), sel(2), sel(2), EditKind.DeleteForward)
        h.record(TextEdit(2, "l", ""), sel(2), sel(2), EditKind.DeleteForward)
        h.record(TextEdit(2, "o", ""), sel(2), sel(2), EditKind.DeleteForward)
        val step = h.undo()
        assertEquals(listOf(TextEdit(2, "", "llo")), step?.edits)
        assertFalse(h.canUndo)
    }

    @Test
    fun composingCoalescesReplaceOfReplace() {
        // 预编辑逐键把同起点区间整体替换：n → ni → 你，净效果一个单元。
        val h = UndoHistory()
        h.record(TextEdit(0, "", "n"), sel(0), sel(1), EditKind.Composing)
        h.record(TextEdit(0, "n", "ni"), sel(1), sel(2), EditKind.Composing)
        h.record(TextEdit(0, "ni", "你"), sel(2), sel(1), EditKind.Composing)
        val step = h.undo()
        assertEquals(listOf(TextEdit(0, "你", "")), step?.edits)
        assertEquals(SelectionState(0, 0), step?.selection)
        assertFalse(h.canUndo)
    }

    @Test
    fun cancelledComposingLeavesNoUnit() {
        val h = UndoHistory()
        h.record(TextEdit(0, "", "n"), sel(0), sel(1), EditKind.Composing)
        h.record(TextEdit(0, "n", ""), sel(1), sel(0), EditKind.Composing)
        assertFalse(h.canUndo) // 净效果为空，不留单元
    }

    @Test
    fun groupRecordsAsSingleUnitAppliedInReverseOnUndo() {
        val h = UndoHistory()
        h.beginGroup()
        h.record(TextEdit(5, "x", ""), sel(5), sel(5), EditKind.Other)
        h.record(TextEdit(2, "y", ""), sel(5), sel(2), EditKind.Other)
        h.endGroup()
        val step = h.undo()
        // 逆序回放：先还原后一笔（offset 2），再还原前一笔（offset 5）。
        assertEquals(listOf(TextEdit(2, "", "y"), TextEdit(5, "", "x")), step?.edits)
        assertEquals(SelectionState(5, 5), step?.selection)
        assertFalse(h.canUndo)
        // redo 按原顺序重放。
        assertEquals(listOf(TextEdit(5, "x", ""), TextEdit(2, "y", "")), h.redo()?.edits)
    }

    @Test
    fun typingDoesNotMergeIntoClosedGroup() {
        val h = UndoHistory()
        h.beginGroup()
        h.record(TextEdit(0, "", "a"), sel(0), sel(1), EditKind.Typing)
        h.endGroup()
        h.record(TextEdit(1, "", "b"), sel(1), sel(2), EditKind.Typing)
        assertEquals(listOf(TextEdit(1, "b", "")), h.undo()?.edits)
        assertTrue(h.canUndo)
    }

    @Test
    fun maxUnitsDropsOldest() {
        val h = UndoHistory(maxUnits = 2)
        h.record(TextEdit(0, "", "a"), sel(0), sel(1), EditKind.Other)
        h.record(TextEdit(1, "", "b"), sel(1), sel(2), EditKind.Other)
        h.record(TextEdit(2, "", "c"), sel(2), sel(3), EditKind.Other)
        assertNotNull(h.undo())
        assertNotNull(h.undo())
        assertNull(h.undo()) // 最老的 a 已被丢弃
    }

    @Test
    fun clearDropsEverything() {
        val h = UndoHistory()
        h.record(TextEdit(0, "", "a"), sel(0), sel(1), EditKind.Typing)
        h.undo()
        h.record(TextEdit(0, "", "b"), sel(0), sel(1), EditKind.Typing)
        h.clear()
        assertFalse(h.canUndo)
        assertFalse(h.canRedo)
    }
}
