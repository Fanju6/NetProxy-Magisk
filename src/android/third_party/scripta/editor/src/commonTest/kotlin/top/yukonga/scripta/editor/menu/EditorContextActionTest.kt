package top.yukonga.scripta.editor.menu

import androidx.compose.ui.unit.IntOffset
import kotlin.test.Test
import kotlin.test.assertEquals

class EditorContextActionTest {

    // --- 可用性规则 --------------------------------------------------------------------------

    @Test
    fun editable_withSelection_allButSelectAllDependOnAllSelected() {
        val av = contextActionAvailability(readOnly = false, hasSelection = true, allSelected = false, docNonEmpty = true)
        assertEquals(ContextActionAvailability(cut = true, copy = true, paste = true, selectAll = true), av)
    }

    @Test
    fun readOnly_gatesOutCutAndPaste_keepsCopyAndSelectAll() {
        val av = contextActionAvailability(readOnly = true, hasSelection = true, allSelected = false, docNonEmpty = true)
        assertEquals(ContextActionAvailability(cut = false, copy = true, paste = false, selectAll = true), av)
    }

    @Test
    fun noSelection_disablesCopyAndCut() {
        val av = contextActionAvailability(readOnly = false, hasSelection = false, allSelected = false, docNonEmpty = true)
        assertEquals(ContextActionAvailability(cut = false, copy = false, paste = true, selectAll = true), av)
    }

    @Test
    fun allSelected_disablesSelectAll() {
        val av = contextActionAvailability(readOnly = false, hasSelection = true, allSelected = true, docNonEmpty = true)
        assertEquals(false, av.selectAll)
    }

    @Test
    fun emptyDoc_disablesSelectAll() {
        val av = contextActionAvailability(readOnly = false, hasSelection = false, allSelected = false, docNonEmpty = false)
        assertEquals(false, av.selectAll)
    }

    // --- 触屏可见项 --------------------------------------------------------------------------

    @Test
    fun touch_editableSelection_showsCutCopyPasteSelectAll() {
        val av = contextActionAvailability(readOnly = false, hasSelection = true, allSelected = false, docNonEmpty = true)
        assertEquals(
            listOf(EditorContextAction.Cut, EditorContextAction.Copy, EditorContextAction.Paste, EditorContextAction.SelectAll),
            visibleTouchActions(av, hasSelection = true),
        )
    }

    @Test
    fun touch_readOnlySelection_showsCopyAndSelectAllOnly() {
        val av = contextActionAvailability(readOnly = true, hasSelection = true, allSelected = false, docNonEmpty = true)
        assertEquals(
            listOf(EditorContextAction.Copy, EditorContextAction.SelectAll),
            visibleTouchActions(av, hasSelection = true),
        )
    }

    @Test
    fun touch_editableCaret_pasteBubbleShowsPasteAndSelectAll() {
        val av = contextActionAvailability(readOnly = false, hasSelection = false, allSelected = false, docNonEmpty = true)
        assertEquals(
            listOf(EditorContextAction.Paste, EditorContextAction.SelectAll),
            visibleTouchActions(av, hasSelection = false),
        )
    }

    @Test
    fun touch_readOnlyEmptyCaret_showsNothing() {
        val av = contextActionAvailability(readOnly = true, hasSelection = false, allSelected = false, docNonEmpty = false)
        assertEquals(emptyList(), visibleTouchActions(av, hasSelection = false))
    }

    @Test
    fun touch_selectionCoveringWholeDoc_dropsSelectAll() {
        val av = contextActionAvailability(readOnly = false, hasSelection = true, allSelected = true, docNonEmpty = true)
        assertEquals(
            listOf(EditorContextAction.Cut, EditorContextAction.Copy, EditorContextAction.Paste),
            visibleTouchActions(av, hasSelection = true),
        )
    }

    // --- 撤销 / 重做可用性 -------------------------------------------------------------------

    @Test
    fun undoRedo_followHistoryState() {
        val av = contextActionAvailability(
            readOnly = false, hasSelection = false, allSelected = false, docNonEmpty = true,
            canUndo = true, canRedo = false,
        )
        assertEquals(true, av.undo)
        assertEquals(false, av.redo)
        assertEquals(true, av.isAvailable(EditorContextAction.Undo))
        assertEquals(false, av.isAvailable(EditorContextAction.Redo))
    }

    @Test
    fun readOnly_disablesUndoRedo() {
        val av = contextActionAvailability(
            readOnly = true, hasSelection = false, allSelected = false, docNonEmpty = true,
            canUndo = true, canRedo = true,
        )
        assertEquals(false, av.undo)
        assertEquals(false, av.redo)
    }

    @Test
    fun touch_neverShowsUndoRedo() {
        val av = contextActionAvailability(
            readOnly = false, hasSelection = true, allSelected = false, docNonEmpty = true,
            canUndo = true, canRedo = true,
        )
        assertEquals(false, visibleTouchActions(av, hasSelection = true).any {
            it == EditorContextAction.Undo || it == EditorContextAction.Redo
        })
    }

    // --- 悬浮条落点：上方优先、near-top 翻下、横向钳制 --------------------------------------

    @Test
    fun toolbar_sitsAboveAnchor_centeredHorizontally() {
        // 锚点中心 x=500、选区顶 y=300；条 200x80、gap=12 → 上方: y = 300−12−80 = 208; x = 500−100 = 400。
        val o = toolbarTopLeft(500f, 300f, 340f, contentW = 200, contentH = 80, windowW = 1000, windowH = 1000, gap = 12, margin = 8)
        assertEquals(IntOffset(400, 208), o)
    }

    @Test
    fun toolbar_flipsBelowWhenNoRoomAbove() {
        // 选区顶 y=40：上方 y = 40−12−80 = −52 < margin(8) → 翻到底部: y = anchorBottom(120)+gap(12) = 132。
        val o = toolbarTopLeft(500f, 40f, 120f, contentW = 200, contentH = 80, windowW = 1000, windowH = 1000, gap = 12, margin = 8)
        assertEquals(132, o.y)
    }

    @Test
    fun toolbar_clampsToWindowHorizontally() {
        // 锚点贴右边 x=990、条宽 200 → 居中会溢出右界，钳到 windowW−contentW−margin = 1000−200−8 = 792。
        val right = toolbarTopLeft(990f, 300f, 340f, contentW = 200, contentH = 80, windowW = 1000, windowH = 1000, gap = 12, margin = 8)
        assertEquals(792, right.x)
        // 贴左边 x=10 → 居中 −90，钳到 margin=8。
        val left = toolbarTopLeft(10f, 300f, 340f, contentW = 200, contentH = 80, windowW = 1000, windowH = 1000, gap = 12, margin = 8)
        assertEquals(8, left.x)
    }
}
