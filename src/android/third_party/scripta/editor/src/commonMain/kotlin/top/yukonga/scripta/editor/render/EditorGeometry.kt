package top.yukonga.scripta.editor.render

import kotlin.math.roundToInt

/** 手柄种类：光标 / 选区起点 / 选区终点。 */
enum class HandleKind { Caret, SelectionStart, SelectionEnd }

/** 视觉行上下移动的落点：目标文档行 [line] 与该行内的视觉行号 [row]（0 基）。 */
data class VisualTarget(val line: Int, val row: Int)

/**
 * 手柄的绘制中心与命中盒，均为屏幕像素、与 Compose 无关，便于单测。
 * [hitContains] 判定触点是否落在命中盒内。
 */
data class HandleGeometry(
    val centerX: Float,
    val centerY: Float,
    val hitLeft: Float,
    val hitTop: Float,
    val hitRight: Float,
    val hitBottom: Float,
) {
    fun hitContains(x: Float, y: Float): Boolean =
        x >= hitLeft && x <= hitRight && y >= hitTop && y <= hitBottom
}

/** 与渲染相关、但不依赖 Compose 的纯几何，便于单测。 */
object EditorGeometry {
    fun gutterDigits(lineCount: Int): Int {
        var digits = 1
        var n = lineCount
        while (n >= 10) {
            n /= 10; digits++
        }
        return maxOf(2, digits)
    }

    // --- 超长「网格行」的等宽算术（M2：不换行下只切可见列窗口，避免 shaping 整行）---

    /**
     * 网格行的可见列窗口 [c0, c1]（含 [marginCols] 余量）：只测量/绘制这段，避免整行 shaping。
     * [scrollX] 横向滚动量、[textAreaWidth] 正文可用宽度、[charW] 等宽字符宽、[lineLength] 该行字符数。
     */
    fun gridVisibleColumns(
        scrollX: Float,
        textAreaWidth: Float,
        charW: Float,
        lineLength: Int,
        marginCols: Int = 8,
    ): IntRange {
        if (charW <= 0f || lineLength <= 0) return 0..0
        val c0 = (scrollX / charW).toInt() - marginCols
        val c1 = ((scrollX + textAreaWidth) / charW).toInt() + marginCols + 1
        return c0.coerceIn(0, lineLength)..c1.coerceIn(0, lineLength)
    }

    /** 网格行：列 → 行内 x 像素（等宽）。 */
    fun gridColumnToX(col: Int, charW: Float): Float = col * charW

    /** 网格行：行内 x 像素 → 列（就近取列边界，钳制到行长）。 */
    fun gridXToColumn(localX: Float, charW: Float, lineLength: Int): Int =
        if (charW <= 0f) 0 else (localX / charW).roundToInt().coerceIn(0, lineLength)

    /**
     * softWrap 下按视觉行上下移动的落点。[curRow] 是光标当前所在视觉行（行内 0 基），[dir] 为 -1 上 / +1 下。
     * 目标视觉行若仍落在本行内则停在本行；否则跨到上一 / 下一文档行的末 / 首视觉行。已在文档首行首视觉行
     * 再上、或末行末视觉行再下时返回 null（不动）。[rowsOf] 给某文档行的视觉行数，须由已测量的 layout 提供
     * 准确值——用估算值会跨行落到错误的视觉行。
     */
    fun visualVerticalTarget(
        curLine: Int,
        curRow: Int,
        dir: Int,
        lineCount: Int,
        rowsOf: (Int) -> Int,
    ): VisualTarget? {
        val targetRow = curRow + dir
        if (targetRow in 0 until rowsOf(curLine)) return VisualTarget(curLine, targetRow)
        return if (dir < 0) {
            if (curLine <= 0) null else VisualTarget(curLine - 1, (rowsOf(curLine - 1) - 1).coerceAtLeast(0))
        } else {
            if (curLine >= lineCount - 1) null else VisualTarget(curLine + 1, 0)
        }
    }

    /**
     * 由光标/端点在屏幕上的矩形（[caretLeft] 以及 [caretTop]/[caretBottom]）推出手柄中心与命中盒。
     * 手柄画在光标正下方（圆心在 [caretBottom] + [radius]）；选区起点手柄略偏左、终点手柄略偏右，
     * 避免手指压住选中文本。命中盒向上延伸到 [caretTop]，让直接抓光标本身也能命中；[slop] 为额外触控外扩。
     */
    fun handleGeometry(
        kind: HandleKind,
        caretLeft: Float,
        caretTop: Float,
        caretBottom: Float,
        radius: Float,
        slop: Float,
    ): HandleGeometry {
        val cx = when (kind) {
            HandleKind.Caret -> caretLeft
            HandleKind.SelectionStart -> caretLeft - radius
            HandleKind.SelectionEnd -> caretLeft + radius
        }
        val cy = caretBottom + radius
        return HandleGeometry(
            centerX = cx,
            centerY = cy,
            hitLeft = cx - (radius + slop),
            hitTop = caretTop,
            hitRight = cx + (radius + slop),
            hitBottom = cy + (radius + slop),
        )
    }
}
