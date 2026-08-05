package top.yukonga.scripta.editor.text

import kotlin.random.Random

/**
 * 纯内存 piece-table：文档 = 片段序列，片段指向只读的 [original] 或 append-only 的 [add] 缓冲。
 * 片段序列用 treap（隐式按 char 数索引的平衡树）维护——split/merge 天然契合「按 offset 分裂片段、
 * 区间删除」，编辑与 offset↔(行,列) 均期望 O(log n)。零 Compose 依赖，便于纯单测，也是 C2（mmap）的核心。
 *
 * 坐标为 UTF-16 char offset；换行为单个 '\n'（调用方需在入口把 CRLF 规整成 LF）。片段内换行定位用
 * 缓冲级 lineStarts（各 '\n' 位置，升序）+ 二分，避免在超长片段里线性扫描。
 */
class PieceTree(text: String = "") {

    private enum class Buf { OG, ADD }

    private var original: String = ""
    private val add = StringBuilder()

    // 各缓冲内 '\n' 的位置（升序）。original 一次算定；add 追加文本时增量补。
    private var ogLF = IntArray(0)
    private var addLF = IntArray(16)
    private var addLFCount = 0

    private val rng = Random(0x5C819A) // 固定种子：treap 优先级确定、可复现（正确性与优先级无关）

    private class Node(
        val buf: Buf,
        val start: Int,
        val len: Int,
        val lf: Int,          // 本片段内换行数
        val priority: Int,
    ) {
        var left: Node? = null
        var right: Node? = null
        var subSize: Int = len // 子树总 char 数
        var subLF: Int = lf    // 子树总换行数
    }

    private var root: Node? = null

    init {
        reset(text)
    }

    // --- 公开：文档信息 -----------------------------------------------------------------------

    val length: Int get() = root?.subSize ?: 0

    val lineCount: Int get() = (root?.subLF ?: 0) + 1

    fun reset(text: String) {
        original = text
        add.setLength(0)
        addLF = IntArray(16); addLFCount = 0
        ogLF = computeLineStarts(text)
        root = if (text.isEmpty()) null
        else Node(Buf.OG, 0, text.length, ogLF.size, priority()).also { updateAgg(it) }
    }

    // --- 公开：读取 ---------------------------------------------------------------------------

    fun getText(): String {
        val sb = StringBuilder(length)
        appendSubtree(root, sb)
        return sb.toString()
    }

    fun substring(offset: Int, len: Int): String {
        if (len <= 0) return ""
        val lo = offset.coerceIn(0, length)
        val hi = (offset + len).coerceIn(0, length)
        if (lo >= hi) return ""
        val sb = StringBuilder(hi - lo)
        appendRange(root, lo, hi, 0, sb)
        return sb.toString()
    }

    fun lineLength(line: Int): Int {
        val l = line.coerceIn(0, lineCount - 1)
        val start = lineStartOffset(l)
        val end = if (l >= lineCount - 1) length else lineStartOffset(l + 1) - 1
        return end - start
    }

    fun lineContent(line: Int): String {
        val l = line.coerceIn(0, lineCount - 1)
        val start = lineStartOffset(l)
        val end = if (l >= lineCount - 1) length else lineStartOffset(l + 1) - 1
        return substring(start, end - start)
    }

    fun offsetAt(position: TextPosition): Int {
        val line = position.line.coerceIn(0, lineCount - 1)
        val col = position.column.coerceIn(0, lineLength(line))
        return lineStartOffset(line) + col
    }

    fun positionAt(offset: Int): TextPosition {
        val off = offset.coerceIn(0, length)
        val line = lineOfOffset(off)
        return TextPosition(line, off - lineStartOffset(line))
    }

    // --- 公开：编辑 ---------------------------------------------------------------------------

    fun insert(offset: Int, text: String) {
        if (text.isEmpty()) return
        val at = offset.coerceIn(0, length)
        val addStart = add.length
        add.append(text)
        // 记录新增文本里的 '\n' 位置（相对 add 缓冲），供后续片段换行定位。
        var i = 0
        while (i < text.length) {
            if (text[i] == '\n') pushAddLF(addStart + i)
            i++
        }
        val node = Node(Buf.ADD, addStart, text.length, countLFIn(Buf.ADD, addStart, text.length), priority())
        updateAgg(node)
        val (l, r) = split(root, at)
        root = merge(merge(l, node), r)
    }

    fun delete(offset: Int, len: Int) {
        if (len <= 0) return
        val at = offset.coerceIn(0, length)
        val n = len.coerceIn(0, length - at)
        if (n == 0) return
        val (l, tmp) = split(root, at)
        val (_, r) = split(tmp, n)
        root = merge(l, r)
    }

    // --- treap split / merge -----------------------------------------------------------------

    /** 把 [t] 分成「前 [k] 个 char」与其余两棵树；[k] 落在某片段内部时就地把该片段拆两半。 */
    private fun split(t: Node?, k: Int): Pair<Node?, Node?> {
        if (t == null) return null to null
        val leftSize = t.left?.subSize ?: 0
        return when {
            k <= leftSize -> {
                val (l, r) = split(t.left, k)
                t.left = r; updateAgg(t)
                l to t
            }

            k >= leftSize + t.len -> {
                val (l, r) = split(t.right, k - leftSize - t.len)
                t.right = l; updateAgg(t)
                t to r
            }

            else -> {
                // k 落在本片段内部：拆成 [start, start+off) 与 [start+off, start+len)。
                // 两半沿用本节点优先级，各接原左/右子树——原节点堆序成立，故拆后仍成立。
                val off = k - leftSize
                val aLF = countLFIn(t.buf, t.start, off)
                val a = Node(t.buf, t.start, off, aLF, t.priority)
                val b = Node(t.buf, t.start + off, t.len - off, t.lf - aLF, t.priority)
                a.left = t.left; a.right = null; updateAgg(a)
                b.left = null; b.right = t.right; updateAgg(b)
                a to b
            }
        }
    }

    private fun merge(l: Node?, r: Node?): Node? {
        if (l == null) return r
        if (r == null) return l
        return if (l.priority >= r.priority) {
            l.right = merge(l.right, r); updateAgg(l); l
        } else {
            r.left = merge(l, r.left); updateAgg(r); r
        }
    }

    private fun updateAgg(n: Node) {
        n.subSize = (n.left?.subSize ?: 0) + n.len + (n.right?.subSize ?: 0)
        n.subLF = (n.left?.subLF ?: 0) + n.lf + (n.right?.subLF ?: 0)
    }

    // --- 遍历 / 定位 -------------------------------------------------------------------------

    private fun appendSubtree(n: Node?, sb: StringBuilder) {
        if (n == null) return
        appendSubtree(n.left, sb)
        sb.appendRange(bufOf(n.buf), n.start, n.start + n.len)
        appendSubtree(n.right, sb)
    }

    /** 把子树内落在绝对区间 [lo, hi) 的字符追加到 [sb]；[base] 为该子树首字符的绝对 offset。 */
    private fun appendRange(n: Node?, lo: Int, hi: Int, base: Int, sb: StringBuilder) {
        if (n == null) return
        val leftSize = n.left?.subSize ?: 0
        val nodeStart = base + leftSize
        val nodeEnd = nodeStart + n.len
        if (lo < nodeStart) appendRange(n.left, lo, hi, base, sb)
        val a = maxOf(lo, nodeStart)
        val b = minOf(hi, nodeEnd)
        if (a < b) {
            val s = n.start + (a - nodeStart)
            sb.appendRange(bufOf(n.buf), s, s + (b - a))
        }
        if (hi > nodeEnd) appendRange(n.right, lo, hi, nodeEnd, sb)
    }

    /** 第 [line] 行起始的绝对 offset。line 0 → 0；否则 = 第 line 个 '\n' 之后。 */
    private fun lineStartOffset(line: Int): Int {
        if (line <= 0) return 0
        var node = root
        var acc = 0
        var need = line // 找第 need 个换行（1 基）之后的位置
        while (node != null) {
            val left = node.left
            val leftLF = left?.subLF ?: 0
            if (need <= leftLF) {
                node = left
            } else {
                need -= leftLF
                if (need <= node.lf) {
                    val leftSize = left?.subSize ?: 0
                    val absLF = nthLFPos(node.buf, node.start, need) // 缓冲内该 '\n' 的绝对位置
                    return acc + leftSize + (absLF - node.start) + 1
                } else {
                    need -= node.lf
                    acc += (left?.subSize ?: 0) + node.len
                    node = node.right
                }
            }
        }
        return length // 行号越界：落到文末
    }

    /** offset 所在行号 = [0, offset) 内的 '\n' 个数。 */
    private fun lineOfOffset(offset: Int): Int {
        var node = root
        var line = 0
        var rem = offset
        while (node != null) {
            val left = node.left
            val leftSize = left?.subSize ?: 0
            if (rem <= leftSize) {
                node = left
            } else {
                line += left?.subLF ?: 0
                rem -= leftSize
                if (rem < node.len) {
                    line += countLFIn(node.buf, node.start, rem)
                    return line
                } else {
                    line += node.lf
                    rem -= node.len
                    node = node.right
                }
            }
        }
        return line
    }

    // --- 缓冲 / 换行辅助 ---------------------------------------------------------------------

    private fun bufOf(b: Buf): CharSequence = if (b == Buf.OG) original else add

    private fun priority(): Int = rng.nextInt()

    private fun pushAddLF(pos: Int) {
        if (addLFCount == addLF.size) addLF = addLF.copyOf(addLF.size * 2)
        addLF[addLFCount++] = pos
    }

    private fun computeLineStarts(text: String): IntArray {
        var count = 0
        for (c in text) if (c == '\n') count++
        val arr = IntArray(count)
        var k = 0
        for (i in text.indices) if (text[i] == '\n') arr[k++] = i
        return arr
    }

    /** 缓冲 [b] 的 [start, start+len) 内 '\n' 个数（二分区间计数）。 */
    private fun countLFIn(b: Buf, start: Int, len: Int): Int {
        if (len <= 0) return 0
        return if (b == Buf.OG) lowerBound(ogLF, ogLF.size, start + len) - lowerBound(ogLF, ogLF.size, start)
        else lowerBound(addLF, addLFCount, start + len) - lowerBound(addLF, addLFCount, start)
    }

    /** 缓冲 [b] 中从 [start] 起第 [m] 个（1 基）'\n' 的绝对位置。 */
    private fun nthLFPos(b: Buf, start: Int, m: Int): Int {
        return if (b == Buf.OG) ogLF[lowerBound(ogLF, ogLF.size, start) + (m - 1)]
        else addLF[lowerBound(addLF, addLFCount, start) + (m - 1)]
    }

    /** arr[0,count) 升序中第一个 ≥ x 的下标（= 严格小于 x 的元素个数）。 */
    private fun lowerBound(arr: IntArray, count: Int, x: Int): Int {
        var lo = 0
        var hi = count
        while (lo < hi) {
            val mid = (lo + hi) ushr 1
            if (arr[mid] < x) lo = mid + 1 else hi = mid
        }
        return lo
    }
}
