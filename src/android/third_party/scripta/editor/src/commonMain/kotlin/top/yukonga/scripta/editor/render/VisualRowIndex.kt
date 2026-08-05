package top.yukonga.scripta.editor.render

/**
 * 软换行模式下的「文档行 ↔ 视觉行」映射。每个文档行占若干视觉行（换行后的行数），默认 1；
 * 可见行被测量后用 [setRows] 更新其真实行数。基于 Fenwick 树（BIT），前缀和/按视觉行定位均为 O(log n)，
 * 因此即便跳到第几十万行也无需测量全文——未测量的行按 1 行估算，滚动时自然收敛。
 *
 * 不换行模式不需要它（每行恒 1 行，用平凡公式即可）。
 */
class VisualRowIndex(lineCount: Int) {

    private var n = lineCount
    private var rowsArr = IntArray(n) { 1 }
    private var tree = IntArray(n + 1)

    init {
        buildTree()
    }

    val lineCount: Int get() = n

    fun rows(line: Int): Int = rowsArr[line]

    fun setRows(line: Int, rows: Int) {
        if (line < 0 || line >= n) return
        val r = rows.coerceAtLeast(1)
        val delta = r - rowsArr[line]
        if (delta == 0) return
        rowsArr[line] = r
        var i = line + 1
        while (i <= n) {
            tree[i] += delta; i += i and (-i)
        }
    }

    /** 视觉行数：行 [0, line) 的行数之和。 */
    fun rowsBefore(line: Int): Int {
        var s = 0
        var x = line.coerceIn(0, n)
        while (x > 0) {
            s += tree[x]; x -= x and (-x)
        }
        return s
    }

    fun totalRows(): Int = rowsBefore(n)

    /** 包含第 [row] 个视觉行的文档行下标（clamp 到 [0, n-1]）。 */
    fun lineAtRow(row: Int): Int {
        if (n == 0) return 0
        var pos = 0
        var remaining = row.coerceAtLeast(0)
        var k = 1
        while (k shl 1 <= n) k = k shl 1
        while (k > 0) {
            val next = pos + k
            if (next <= n && tree[next] <= remaining) {
                pos = next
                remaining -= tree[next]
            }
            k = k shr 1
        }
        return pos.coerceIn(0, n - 1)
    }

    /**
     * 行结构变化：从 [from] 起的 [oldLines] 行被 [newLines] 行取代。编辑区间外的已测量行数原样保留
     * （前段不动、尾段平移），新行以 1 行估算落地、可见后经 [setRows] 收敛。这样行数一变不必整表重建：
     * 重建会把全文档打回 1 行估算，内容总高度骤变、视口跳动。O(n) 数组拷贝 + 重建树——保的是测量数据
     * 不是渐进复杂度；行数变化（回车/删行）远比击键稀疏，线性拷贝可担。
     */
    fun splice(from: Int, oldLines: Int, newLines: Int) {
        val f = from.coerceIn(0, n)
        val removed = oldLines.coerceIn(0, n - f)
        val inserted = newLines.coerceAtLeast(0)
        val m = n - removed + inserted
        val next = IntArray(m)
        rowsArr.copyInto(next, 0, 0, f)
        next.fill(1, f, f + inserted)
        rowsArr.copyInto(next, f + inserted, f + removed, n)
        rowsArr = next
        n = m
        buildTree()
    }

    private fun buildTree() {
        tree = IntArray(n + 1)
        for (i in 1..n) {
            tree[i] += rowsArr[i - 1]
            val parent = i + (i and (-i))
            if (parent <= n) tree[parent] += tree[i]
        }
    }
}
