package top.yukonga.scripta.editor.render

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class EditorGeometryTest {
    @Test
    fun gutterDigitsHasMinimumTwo() {
        assertEquals(2, EditorGeometry.gutterDigits(1))
        assertEquals(2, EditorGeometry.gutterDigits(9))
        assertEquals(2, EditorGeometry.gutterDigits(99))
        assertEquals(3, EditorGeometry.gutterDigits(100))
        assertEquals(4, EditorGeometry.gutterDigits(1234))
    }

    @Test
    fun caretHandleSitsBelowCaretCentered() {
        // 光标屏幕矩形：x=100, top=0, bottom=20；radius=8, slop=6
        val g = EditorGeometry.handleGeometry(HandleKind.Caret, 100f, 0f, 20f, 8f, 6f)
        assertEquals(100f, g.centerX)          // 光标手柄水平居中
        assertEquals(28f, g.centerY)           // bottom(20) + radius(8)
        assertEquals(86f, g.hitLeft)           // cx - (radius+slop) = 100-14
        assertEquals(0f, g.hitTop)             // 命中盒上沿到光标顶
        assertEquals(114f, g.hitRight)
        assertEquals(42f, g.hitBottom)         // cy + (radius+slop) = 28+14
    }

    @Test
    fun selectionHandlesLeanOutward() {
        val start = EditorGeometry.handleGeometry(HandleKind.SelectionStart, 100f, 0f, 20f, 8f, 0f)
        val end = EditorGeometry.handleGeometry(HandleKind.SelectionEnd, 100f, 0f, 20f, 8f, 0f)
        assertEquals(92f, start.centerX)       // 起点手柄偏左 radius
        assertEquals(108f, end.centerX)        // 终点手柄偏右 radius
    }

    @Test
    fun hitContainsRespectsBox() {
        val g = EditorGeometry.handleGeometry(HandleKind.Caret, 100f, 0f, 20f, 8f, 6f)
        assertTrue(g.hitContains(100f, 28f))   // 圆心
        assertTrue(g.hitContains(100f, 1f))    // 光标线附近（盒内上部）
        assertFalse(g.hitContains(100f, 60f))  // 远在盒下方
        assertFalse(g.hitContains(130f, 28f))  // 远在盒右侧
    }

    // --- B5: softWrap 视觉行上下移动落点 ---

    // 文档：行0 折成 3 视觉行，行1 折成 1 行，行2 折成 2 行。
    private val rows = intArrayOf(3, 1, 2)
    private fun target(line: Int, row: Int, dir: Int) =
        EditorGeometry.visualVerticalTarget(line, row, dir, rows.size) { rows[it] }

    @Test
    fun visualDownStaysWithinWrappedLine() {
        assertEquals(VisualTarget(0, 1), target(0, 0, +1)) // 行0 视觉行 0 -> 1，仍在本行
        assertEquals(VisualTarget(0, 2), target(0, 1, +1))
    }

    @Test
    fun visualDownCrossesToNextLineFirstRow() {
        assertEquals(VisualTarget(1, 0), target(0, 2, +1)) // 行0 末视觉行 -> 行1 首视觉行
    }

    @Test
    fun visualUpCrossesToPrevLineLastRow() {
        assertEquals(VisualTarget(0, 2), target(1, 0, -1)) // 行1 首行 -> 行0 末视觉行（2）
    }

    @Test
    fun visualUpStaysWithinWrappedLine() {
        assertEquals(VisualTarget(0, 1), target(0, 2, -1))
        assertEquals(VisualTarget(0, 0), target(0, 1, -1))
    }

    @Test
    fun visualUpAtDocumentStartReturnsNull() {
        assertEquals(null, target(0, 0, -1)) // 文档首行首视觉行再上 -> 不动
    }

    @Test
    fun visualDownAtDocumentEndReturnsNull() {
        assertEquals(null, target(2, 1, +1)) // 末行末视觉行再下 -> 不动
    }

    // --- M2: 网格行等宽算术 ---

    @Test
    fun gridVisibleColumnsWindowsAroundScroll() {
        // charW=10, 视口正文宽 100 -> 约 10 列可见；margin=2
        val r = EditorGeometry.gridVisibleColumns(scrollX = 300f, textAreaWidth = 100f, charW = 10f, lineLength = 1000, marginCols = 2)
        assertEquals(28, r.first)  // 300/10 - 2
        assertEquals(43, r.last)   // 400/10 + 2 + 1
    }

    @Test
    fun gridVisibleColumnsClampToLineBounds() {
        val r = EditorGeometry.gridVisibleColumns(scrollX = 0f, textAreaWidth = 100f, charW = 10f, lineLength = 5, marginCols = 2)
        assertEquals(0, r.first)   // -2 -> 0
        assertEquals(5, r.last)    // 13 -> 5（行长）
    }

    @Test
    fun gridVisibleColumnsDegenerateCharW() {
        val r = EditorGeometry.gridVisibleColumns(0f, 100f, 0f, 1000, 2)
        assertEquals(0, r.first); assertEquals(0, r.last) // charW=0 兜底
    }

    @Test
    fun gridColumnXRoundTrip() {
        assertEquals(50f, EditorGeometry.gridColumnToX(5, 10f))
        assertEquals(5, EditorGeometry.gridXToColumn(52f, 10f, 100)) // round(5.2)
        assertEquals(6, EditorGeometry.gridXToColumn(56f, 10f, 100)) // round(5.6)
        assertEquals(100, EditorGeometry.gridXToColumn(9999f, 10f, 100)) // 钳制上界
        assertEquals(0, EditorGeometry.gridXToColumn(-5f, 10f, 100))     // 钳制下界
    }
}
