package top.yukonga.scripta.editor.render

import kotlin.test.Test
import kotlin.test.assertEquals

class VisualRowIndexTest {
    @Test
    fun defaultsToOneRowPerLine() {
        val idx = VisualRowIndex(5)
        assertEquals(5, idx.totalRows())
        assertEquals(0, idx.rowsBefore(0))
        assertEquals(3, idx.rowsBefore(3))
        assertEquals(1, idx.rows(2))
    }

    @Test
    fun setRowsUpdatesPrefixesAndTotal() {
        val idx = VisualRowIndex(5)
        idx.setRows(1, 3) // line 1 wraps to 3 visual rows
        assertEquals(7, idx.totalRows()) // 1 + 3 + 1 + 1 + 1
        assertEquals(0, idx.rowsBefore(0))
        assertEquals(1, idx.rowsBefore(1))
        assertEquals(4, idx.rowsBefore(2)) // 1 + 3
        assertEquals(5, idx.rowsBefore(3)) // 1 + 3 + 1
    }

    @Test
    fun lineAtRowFindsContainingLine() {
        val idx = VisualRowIndex(5)
        idx.setRows(1, 3) // visual rows: L0=[0], L1=[1,2,3], L2=[4], L3=[5], L4=[6]
        assertEquals(0, idx.lineAtRow(0))
        assertEquals(1, idx.lineAtRow(1))
        assertEquals(1, idx.lineAtRow(3))
        assertEquals(2, idx.lineAtRow(4))
        assertEquals(3, idx.lineAtRow(5))
        assertEquals(4, idx.lineAtRow(6))
        assertEquals(4, idx.lineAtRow(999)) // clamp
    }

    @Test
    fun singleLine() {
        val idx = VisualRowIndex(1)
        assertEquals(1, idx.totalRows())
        assertEquals(0, idx.lineAtRow(0))
        idx.setRows(0, 4)
        assertEquals(4, idx.totalRows())
        assertEquals(0, idx.lineAtRow(3))
    }

    @Test
    fun spliceInsertKeepsMeasuredRowsOutsideTheEdit() {
        val idx = VisualRowIndex(5)
        idx.setRows(0, 2)
        idx.setRows(2, 3)
        idx.setRows(4, 4)
        idx.splice(2, 1, 2) // 在 L2 回车：1 行变 2 行
        assertEquals(6, idx.lineCount)
        assertEquals(2, idx.rows(0)) // 编辑点之前保留测量值
        assertEquals(1, idx.rows(2)) // 替换出的新行回 1 行估算
        assertEquals(1, idx.rows(3))
        assertEquals(4, idx.rows(5)) // 编辑点之后平移并保留测量值
        assertEquals(2 + 1 + 1 + 1 + 1 + 4, idx.totalRows())
    }

    @Test
    fun spliceRemoveKeepsMeasuredRowsOutsideTheEdit() {
        val idx = VisualRowIndex(5)
        idx.setRows(0, 2)
        idx.setRows(2, 5)
        idx.setRows(4, 4)
        idx.splice(1, 3, 1) // L1..L3 三行并成 1 行
        assertEquals(3, idx.lineCount)
        assertEquals(2, idx.rows(0))
        assertEquals(1, idx.rows(1))
        assertEquals(4, idx.rows(2))
        assertEquals(7, idx.totalRows())
    }

    @Test
    fun splicePrefixSumsAndRowLookupStayConsistent() {
        val idx = VisualRowIndex(4)
        idx.setRows(1, 2)
        idx.splice(2, 1, 3) // L2 替换为 3 行
        // rows: L0=1, L1=2, L2..L4=1(新), L5=1(原 L3)
        assertEquals(6, idx.lineCount)
        assertEquals(3, idx.rowsBefore(2))
        assertEquals(1, idx.lineAtRow(1))
        assertEquals(1, idx.lineAtRow(2))
        assertEquals(5, idx.lineAtRow(6))
        assertEquals(7, idx.totalRows())
    }

    @Test
    fun spliceAtDocumentEndAppendsLines() {
        val idx = VisualRowIndex(3)
        idx.setRows(0, 2)
        idx.splice(2, 1, 2) // 文末行回车
        assertEquals(4, idx.lineCount)
        assertEquals(2, idx.rows(0))
        assertEquals(5, idx.totalRows())
    }

    @Test
    fun spliceClampsOutOfRangeArguments() {
        val idx = VisualRowIndex(3)
        idx.setRows(0, 2)
        idx.splice(2, 99, 1) // 删除数越过尾部 → 只删到尾
        assertEquals(3, idx.lineCount) // 3 - 1 + 1
        assertEquals(2, idx.rows(0))
        assertEquals(4, idx.totalRows())
    }
}
