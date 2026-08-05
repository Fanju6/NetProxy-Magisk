package top.yukonga.scripta.editor.render

import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.tween
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.dropShadow
import androidx.compose.ui.draw.innerShadow
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.geometry.RoundRect
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.drawscope.clipPath
import androidx.compose.ui.graphics.drawscope.translate
import androidx.compose.ui.graphics.drawscope.withTransform
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.graphics.shadow.Shadow
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.text.TextLayoutResult
import androidx.compose.ui.text.TextMeasurer
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.drawText
import androidx.compose.ui.unit.Density
import androidx.compose.ui.unit.DpOffset
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Popup
import androidx.compose.ui.window.PopupProperties
import top.yukonga.scripta.editor.EditorColors
import top.yukonga.scripta.editor.EditorEngine
import top.yukonga.scripta.editor.LineNumberMode
import top.yukonga.scripta.editor.LruCache
import top.yukonga.scripta.editor.find.FindSpan
import top.yukonga.scripta.editor.highlight.HighlightCache
import top.yukonga.scripta.editor.highlight.SyntaxColors
import top.yukonga.scripta.editor.highlight.clipSpansToWindow
import top.yukonga.scripta.editor.highlight.highlightedText
import top.yukonga.scripta.editor.text.BracketPair
import top.yukonga.scripta.editor.text.TextPosition
import top.yukonga.scripta.editor.text.TextRange
import kotlin.math.PI
import kotlin.math.acos
import kotlin.math.atan2
import kotlin.math.cos
import kotlin.math.roundToInt
import kotlin.math.sin

/** 网格行可见列切片缓存的键：行 + 量化后的列窗口 [start,end) + 文档版本（编辑即失效）。 */
private data class GridSliceKey(val line: Int, val start: Int, val end: Int, val version: Int)

/**
 * 网格切片测量（主画布与放大镜共用）：可见列窗口切片 + 裁剪到窗口的着色段。colorOnly——网格等宽
 * 算术假设基础字宽，字重会改度量、颜色不会。超过分词上限的行 spans 为空，自然退回纯色。
 */
private fun measureGridSlice(
    engine: EditorEngine,
    textMeasurer: TextMeasurer,
    textStyle: TextStyle,
    syntax: SyntaxColors,
    highlightCache: HighlightCache?,
    line: Int,
    from: Int,
    until: Int,
): TextLayoutResult {
    val slice = engine.buffer.textInRange(TextRange(TextPosition(line, from), TextPosition(line, until)))
    val spans = highlightCache?.spansForLine(line) { engine.buffer.lineText(it) } ?: emptyList()
    val clipped = clipSpansToWindow(spans, from, until)
    return if (clipped.isEmpty()) textMeasurer.measure(slice, textStyle, softWrap = false)
    else textMeasurer.measure(highlightedText(slice, clipped, syntax, colorOnly = true), textStyle, softWrap = false)
}

/** 网格行切片列窗口量化粒度：窗口左右对齐到 32 列倍数，横滚每 32 列才换一次 key、其余帧命中缓存。 */
private const val GRID_SLICE_QUANTUM = 32

/**
 * 泪滴手柄路径：尖端 ([tipX],[tipY]) 贴光标底，body 圆（半径 [r]）沿单位向量 ([ux],[uy]) 从尖端向外下方悬挂，
 * 尖端到圆心距 [l]（须 > r，越大越尖）。两侧为「尖端到圆的切线」、外缘为圆的大弧，合成有尖角的水滴形。
 * 光标手柄尖朝正上、选区起点尖朝右上（body 左下）、终点尖朝左上（body 右下），起点/终点为镜像对。
 */
private fun buildTeardropPath(tipX: Float, tipY: Float, ux: Float, uy: Float, r: Float, l: Float): Path {
    val cx = tipX + l * ux
    val cy = tipY + l * uy
    val gamma = acos((r / l).toDouble().coerceIn(-1.0, 1.0)) // 圆心处的半张角（切线与圆心-尖端连线夹角）
    val phi = atan2((tipY - cy).toDouble(), (tipX - cx).toDouble()) // 圆心 → 尖端 方向（rad）
    val t1 = phi + gamma // 一侧切点的圆心角
    val t1x = (cx + r * cos(t1)).toFloat()
    val t1y = (cy + r * sin(t1)).toFloat()
    val rad2deg = 180.0 / PI
    val bounds = Rect(cx - r, cy - r, cx + r, cy + r)
    return Path().apply {
        moveTo(tipX, tipY)
        lineTo(t1x, t1y)
        // 从 t1 切点起，扫过「背向尖端」的大弧 (2π − 2γ) 到另一切点，close 收回尖端 → 实心水滴。
        arcTo(bounds, (t1 * rad2deg).toFloat(), ((2 * PI - 2 * gamma) * rad2deg).toFloat(), forceMoveTo = false)
        close()
    }
}

/**
 * 只绘制可见行的画布。从 [firstVisibleLine] 向下按 [lineTopPx] 走，直到超出视口。行高取自各行 layout
 * （换行模式下一行可占多视觉行），因此对换行/不换行统一处理。
 *
 * 行号横向锚定见 [lineNumberMode]：正文/光标/手柄画完后统一铺不透明 gutter 条 + 行号，滚进来的正文透不进
 * 行号区。[LineNumberMode.PinnedToScreen] 的 gutter/行号钉屏幕左侧；[LineNumberMode.PinnedToLine] 的 gutter/
 * 行号随内容横移、滚到右侧一并从左边滑出。
 *
 * 每行文字/光标/选区/预编辑都按 `refBaselinePx - layout.firstBaseline` 做基线平移，使所有行（中/英/混排）
 * 的基线落在同一水平线，抵消 CJK/拉丁字体度量差导致的整行基线偏移。
 *
 * 超长「网格行」（[isGridLine] 为真）不整行 shaping：只测量/绘制落在视口内的可见列窗口切片，几何（光标/
 * 选区/手柄）用等宽算术（[charW] + 参考字符垂直度量 [gridRefBaseline]/[gridRefCursorTop]/[gridRefCursorBottom]）。
 *
 * 双指缩放预览（[previewScale] != 1f）：draw 阶段用运行态仿射 (s=[previewScale], tx=[previewTx], ty=[previewTy]) 变换正文——
 * screenX = s·px + tx、screenY = s·py + ty。手势由上层「绕当前双指中点缩放 + 中点位移平移」逐帧增量累积 → **四向自由跟手**；
 * **gutter/行号**横向钉左（tx 恒 0、只随字号缩放、不横移），纵向与正文一致（同 ty）。字号未变、layout 未重排，Skia 按缩放后度量
 * 重栅格化 → 文字清晰、零重排；缩小 / 平移使可见带外扩时 [firstVisibleLine] 传入外扩量补齐露出的行。变换可越界（进空白），
 * rubber-band / 松手 settle 由上层处理，本层不钳制。松手 commitZoom 把 (s,tx,ty) 折进真实 scroll（自相似 + TextMotion ⇒ 逐像素接续）。
 */
@Composable
fun EditorCanvas(
    engine: EditorEngine,
    colors: EditorColors,
    textMeasurer: TextMeasurer,
    textStyle: TextStyle,
    numberStyle: TextStyle,
    lineHeightPx: Float,
    gutterWidthPx: Float,
    padXPx: Float,
    lineNumberMode: LineNumberMode,
    scrollX: () -> Float,
    scrollY: () -> Float,
    firstVisibleLine: (Float) -> Int,
    lineTopPx: (Int) -> Float,
    refBaselinePx: Float,
    caretHandleVisible: () -> Boolean,
    selectionHandlesVisible: () -> Boolean = { true },
    handleRadiusPx: Float,
    layoutFor: (Int) -> TextLayoutResult?,
    charW: Float,
    isGridLine: (Int) -> Boolean,
    gridRefBaseline: Float,
    gridRefCursorTop: Float,
    gridRefCursorBottom: Float,
    findSpansForLine: (Int) -> List<FindSpan> = { emptyList() },
    activeFindIndex: () -> Int = { -1 },
    // 匹配括号（光标邻接括号时的两个单字符位置）：draw 读 derivedState——光标动只重绘、不重组。
    bracketMatch: () -> BracketPair? = { null },
    // 语法高亮状态链（null = 无高亮）：网格长行的可见列切片据此取着色段。draw 恒在同帧组合之后，
    // 组合里的 ensureHighlightFresh 已截断状态链，这里直接取即可。
    highlightCache: HighlightCache? = null,
    previewScale: () -> Float = { 1f },
    previewTx: () -> Float = { 0f },
    previewTy: () -> Float = { 0f },
    modifier: Modifier = Modifier,
) {
    // 行号 layout 缓存：行号只依赖行下标与 numberStyle，draw 里逐帧重测会击穿 TextMeasurer 仅 8 条的
    // 内部缓存（可见行常几十行、必然大量 miss）。numberStyle 变化（字号/配色）时整表失效；有界 LRU
    // 超上限淘汰最久未用，避免整表 clear() 把可见行号一并丢弃。缓存与逐帧重测输出一致，仅省掉重复布局。
    val numberLayoutCache = remember(numberStyle) { LruCache<Int, TextLayoutResult>(4096) }
    // 网格行可见列切片缓存：横滚/纵向 fling 时避免每帧 textInRange 新建切片 String + measure 重 shaping。
    // key 含文档版本（编辑即失效），列窗口量化到 32 列 → 窗口内滚动/闪烁/拖选帧全命中，横滚每 32 列才重测一次。
    // 整表失效 key：textStyle（字号/前景）之外还要 colors.syntax 与 highlightCache——切片现在带语法着色，
    // 仅语法色变化时 textStyle 内容相同（remember 按 equals 比），换插件时 version 也不动，都得单列。
    val gridSliceCache = remember(textStyle, colors.syntax, highlightCache) { LruCache<GridSliceKey, TextLayoutResult>(256) }
    // 泪滴手柄路径按 kind 预建缓存（尖端在原点、body 沿方向悬挂）：只依赖 handleRadiusPx（固定 dp），绘制时每帧只 translate
    // 到光标底 + drawPath，免去逐帧新建 Path + acos/atan2 + arcTo 弧线细分（drawHandle 每帧对每个可见手柄都画一次）。
    val diag = 0.70710677f
    val caretTeardrop = remember(handleRadiusPx) { buildTeardropPath(0f, 0f, 0f, 1f, handleRadiusPx, handleRadiusPx * 1.4f) }
    val startTeardrop = remember(handleRadiusPx) { buildTeardropPath(0f, 0f, -diag, diag, handleRadiusPx, handleRadiusPx * 1.4f) }
    val endTeardrop = remember(handleRadiusPx) { buildTeardropPath(0f, 0f, diag, diag, handleRadiusPx, handleRadiusPx * 1.4f) }
    Canvas(modifier) {
        // 在 draw 阶段读取滚动量：滚动只触发本画布重绘，不再让上层每滚 1px 重组。
        val sX = scrollX()
        val sY = scrollY()
        // 缩放预览的运行态仿射在 draw 阶段读：previewScale/previewTx/previewTy 每事件变、仅重绘（零重组/零重排）。(1,0,0) 时退化为原状。
        // screenX = s·px + hTranslateText、screenY = s·py + vTranslate（px/py 为相对 sX/sY 的内容坐标）。四向自由跟手由上层逐帧累积
        // （可越界，rubber-band / 松手 settle 在上层做）→ 本层不钳制、直接用。
        val s = previewScale()
        val hTranslateText = previewTx()
        val vTranslate = previewTy()
        val preTop = -vTranslate / s // 可见带上界（预缩放屏幕 y）：由 screenY=0 反解 py = −vTranslate/s，作循环起点/可见判定基准
        val preBottom = (size.height - vTranslate) / s // 可见带下界：由 screenY=size.height 反解，作循环终止边界（over-draw）
        // 当前行整行高亮的预缩放矩形：经横向变换 screenX=s·px+hTranslateText 后仍铺满视口宽 [0, size.width]——否则缩小(s<1)时
        // 直接用 size.width 会被 s 缩窄、右侧铺不满（「高亮不到底」）。s==1 时 hlLeft=0、hlWidth=size.width，与原状一致。
        val hlLeft = -hTranslateText / s
        val hlWidth = size.width / s
        val bufVersion = engine.buffer.version // 网格切片缓存键用；编辑变更即让旧切片失效
        val pinnedToScreen = lineNumberMode == LineNumberMode.PinnedToScreen
        // gutter 与行号的横向偏移：固定于屏幕钉在左侧（0）；固定于行随内容横移（sX），gutter 底色条连同行号
        // 一起向左滑出。两种模式都有区别于正文区的 gutter 底色（[colors.gutterBackground]）。
        val gutterScroll = if (pinnedToScreen) 0f else sX

        // 背景铺满整视口，**不参与缩放**——否则缩小预览时缩过的背景盖不满视口、露出上一帧/黑边。
        drawRect(colors.background, topLeft = Offset.Zero, size = size)

        // 逐行收集的行号（行, 预缩放屏幕顶 y）：正文块填充、gutter 块绘制（两块横向变换不同，故提到块外共享）。
        val deferredNumbers = ArrayList<Pair<Int, Float>>()

        // 正文/选区/预编辑/手柄：横向绕焦点（hTranslateText）+ 纵向绕焦点（vTranslate）→ screenX=s·px+hTranslateText、
        // screenY=s·py+vTranslate。放大时焦点左侧正文左移滑到 gutter 后、上方内容上移、焦点处停在指下。scale==1 且两平移=0 时恒等。
        withTransform({ translate(hTranslateText, vTranslate); scale(s, s, Offset.Zero) }) {
            val sel = engine.selection
            val comp = engine.composing
            val textX = gutterWidthPx + padXPx - sX
            val lineCount = engine.buffer.lineCount

            fun drawLineNumber(line: Int, top: Float) {
                deferredNumbers.add(line to top)
            }

            var line = firstVisibleLine(preTop).coerceIn(0, (lineCount - 1).coerceAtLeast(0))
            while (line < lineCount) {
                val top = lineTopPx(line) - sY
                if (top >= preBottom) break

                if (isGridLine(line)) {
                    // 超长网格行：只测量/绘制可见列窗口，几何用等宽算术（单视觉行，行高 = lineHeightPx）。
                    val h = lineHeightPx
                    if (top + h > preTop) {
                        val lineLen = engine.buffer.lineLength(line)
                        val textTop = top + (refBaselinePx - gridRefBaseline)
                        if (sel.isEmpty && sel.start.line == line) {
                            drawRect(colors.currentLine, topLeft = Offset(hlLeft, top), size = Size(hlWidth, h))
                        }
                        // 查找命中底色（选区之下）：网格行走等宽算术。当前命中用强调色。
                        val findSpans = findSpansForLine(line)
                        if (findSpans.isNotEmpty()) {
                            val act = activeFindIndex()
                            for (sp in findSpans) {
                                val fS = sp.startCol.coerceIn(0, lineLen)
                                val fE = sp.endCol.coerceIn(0, lineLen)
                                if (fE > fS) drawRect(
                                    if (sp.matchIndex == act) colors.findMatchActive else colors.findMatch,
                                    topLeft = Offset(textX + fS * charW, top),
                                    size = Size((fE - fS) * charW, h)
                                )
                            }
                        }
                        // 匹配括号底色：网格行同走等宽算术（单字符格）。
                        bracketMatch()?.let { bm ->
                            if (bm.open.line == line && bm.open.column < lineLen) {
                                drawRect(colors.bracketMatch, topLeft = Offset(textX + bm.open.column * charW, top), size = Size(charW, h))
                            }
                            if (bm.close.line == line && bm.close.column < lineLen) {
                                drawRect(colors.bracketMatch, topLeft = Offset(textX + bm.close.column * charW, top), size = Size(charW, h))
                            }
                        }
                        if (!sel.isEmpty && line >= sel.start.line && line <= sel.end.line) {
                            val cS = if (line == sel.start.line) sel.start.column else 0
                            val cE = if (line == sel.end.line) sel.end.column else lineLen
                            if (cE > cS) {
                                drawRect(colors.selection, topLeft = Offset(textX + cS * charW, top), size = Size((cE - cS) * charW, h))
                            }
                            if (line != sel.end.line) {
                                drawRect(
                                    colors.selection,
                                    topLeft = Offset(textX + lineLen * charW, top),
                                    size = Size(lineHeightPx * 0.4f, h)
                                )
                            }
                        }
                        // 正文：只切落在视口内的列窗口测量，避免 shaping 整行。列窗口量化到 32 列倍数（左取整、右上取整
                        // 钳到行长），使小幅横滚落在同一量化窗口 → 命中 gridSliceCache，仅每跨 32 列才重测；多切的 ≤64
                        // 个字符落在视口外、由 clipToBounds 裁掉，视觉无差。
                        // 可见列窗口按预览缩放补偿：缩小(s<1)时屏上可见内容宽 = size.width/s（更宽），左缘内容 x 随焦点缩放偏移
                        // hTranslateText/s；否则切片覆盖不到缩小后新露出的右侧列 → 长行右段缺失。s==1 退化为原状（仅切文本区）。
                        val cols = if (s == 1f) {
                            EditorGeometry.gridVisibleColumns(
                                sX,
                                (size.width - gutterWidthPx - padXPx * 2).coerceAtLeast(1f),
                                charW,
                                lineLen
                            )
                        } else {
                            EditorGeometry.gridVisibleColumns(sX - hTranslateText / s, size.width / s, charW, lineLen)
                        }
                        if (cols.last > cols.first) {
                            val q = GRID_SLICE_QUANTUM
                            val qStart = (cols.first / q) * q
                            val qEnd = (((cols.last + q - 1) / q) * q).coerceAtMost(lineLen)
                            val sliceLayout = gridSliceCache.getOrPut(GridSliceKey(line, qStart, qEnd, bufVersion)) {
                                measureGridSlice(engine, textMeasurer, textStyle, colors.syntax, highlightCache, line, qStart, qEnd)
                            }
                            // 不传 color：切片自带语法 span 色，未覆盖处用 textStyle 的前景色（同普通行画法）。
                            drawText(sliceLayout, topLeft = Offset(textX + qStart * charW, textTop))
                        }
                        drawLineNumber(line, top)
                    }
                    line++
                    continue
                }

                val layout = layoutFor(line)
                if (layout == null) {
                    line++; continue
                }
                val h = layout.size.height.toFloat()
                val textTop = top + (refBaselinePx - layout.firstBaseline) // 基线对齐后的绘制顶
                if (top + h > preTop) {
                    // 当前行淡色高亮（无选择时）：铺满整行宽（含 gutter 区）。固定模式下左段会被文末 gutter 条盖住、
                    // 视觉不变；跟随模式下 gutter 条随内容滚走，整行仍均匀高亮、左侧不留缺口。
                    if (sel.isEmpty && sel.start.line == line) {
                        drawRect(colors.currentLine, topLeft = Offset(hlLeft, top), size = Size(hlWidth, h))
                    }
                    // 查找命中底色（选区之下）：range path 与选区同式、锚固定行栅格 top。当前命中用强调色。
                    val findSpans = findSpansForLine(line)
                    if (findSpans.isNotEmpty()) {
                        val act = activeFindIndex()
                        val lineLen = engine.buffer.lineLength(line)
                        for (sp in findSpans) {
                            val fS = sp.startCol.coerceIn(0, lineLen)
                            val fE = sp.endCol.coerceIn(0, lineLen)
                            if (fE > fS) {
                                val p = layout.getPathForRange(fS, fE)
                                p.translate(Offset(textX, top))
                                drawPath(p, if (sp.matchIndex == act) colors.findMatchActive else colors.findMatch)
                            }
                        }
                    }
                    // 匹配括号底色：单字符 range path，与查找同式（softWrap 折行几何由 layout 兜住）。
                    bracketMatch()?.let { bm ->
                        val lineLen = engine.buffer.lineLength(line)
                        fun drawBracketAt(col: Int) {
                            if (col < lineLen) {
                                val p = layout.getPathForRange(col, col + 1)
                                p.translate(Offset(textX, top))
                                drawPath(p, colors.bracketMatch)
                            }
                        }
                        if (bm.open.line == line) drawBracketAt(bm.open.column)
                        if (bm.close.line == line) drawBracketAt(bm.close.column)
                    }
                    // 选区底色纵向锚到固定行栅格 top（非 textTop）：须行行相接铺满不重叠——跟随 textTop（各行基线平移不一）会交叠。
                    if (!sel.isEmpty && line >= sel.start.line && line <= sel.end.line) {
                        val lineLen = engine.buffer.lineLength(line)
                        val cS = if (line == sel.start.line) sel.start.column else 0
                        val cE = if (line == sel.end.line) sel.end.column else lineLen
                        if (cE > cS) {
                            val path = layout.getPathForRange(cS, cE)
                            path.translate(Offset(textX, top)) // getPathForRange 的行盒为 [0,lineHeight] → 平移到 [top, top+行高]
                            drawPath(path, colors.selection)
                        }
                        if (line != sel.end.line) {
                            val cr = layout.getCursorRect(lineLen)
                            drawRect(
                                colors.selection,
                                topLeft = Offset(textX + cr.left, top),
                                size = Size(lineHeightPx * 0.4f, lineHeightPx)
                            )
                        }
                    }
                    // 正文（含换行后的多视觉行）。不传 color：layout 自带基础色 + 语法高亮 span 色。
                    drawText(layout, topLeft = Offset(textX, textTop))
                    // 行号（基线对齐到正文基线）；命中缓存则跳过重测（LRU 自动淘汰，无需手动清空）
                    drawLineNumber(line, top)
                }
                line++
            }

            // 预编辑下划线
            comp?.let { c ->
                val cLine = c.start.line
                if (isGridLine(cLine)) {
                    val textTop = lineTopPx(cLine) - sY + (refBaselinePx - gridRefBaseline)
                    val len = engine.buffer.lineLength(cLine)
                    val xS = textX + c.start.column.coerceIn(0, len) * charW
                    val xE = textX + c.end.column.coerceIn(0, len) * charW
                    val y = textTop + gridRefCursorBottom - 2f
                    drawLine(colors.cursor, Offset(xS, y), Offset(xE, y), strokeWidth = 3f)
                } else {
                    val layout = layoutFor(cLine)
                    if (layout != null) {
                        val textTop = lineTopPx(cLine) - sY + (refBaselinePx - layout.firstBaseline)
                        val len = engine.buffer.lineLength(cLine)
                        val startRect = layout.getCursorRect(c.start.column.coerceIn(0, len))
                        val endRect = layout.getCursorRect(c.end.column.coerceIn(0, len))
                        if (startRect.top == endRect.top) { // 同一视觉行
                            val y = textTop + startRect.bottom - 2f
                            drawLine(colors.cursor, Offset(textX + startRect.left, y), Offset(textX + endRect.left, y), strokeWidth = 3f)
                        }
                    }
                }
            }

            // 光标不在此画：拆到独立的 [CursorOverlay] 图层，闪烁只切该层 alpha、不重放整块正文画布（见 P10）。

            // 拖动手柄（泪滴）：尖端贴光标底、body 圆朝外下方悬挂作抓取区。选区两端按 selectionHandlesVisible 控制、
            // 光标手柄按 caretHandleVisible 控制（鼠标交互时二者均隐藏——桌面/鼠标不需要触摸取词手柄）。
            // 网格行用等宽算术定光标矩形，其余走该行 layout。缩放预览期（s != 1f）隐藏——避免手柄随内容缩放显得突兀，
            // 松手提交后按新布局重新出现，位置正确。命中盒仍由 EditorGeometry.handleGeometry（上层命中侧）给出、覆盖此泪滴。
            fun drawHandle(kind: HandleKind, pos: TextPosition) {
                val caretLeft: Float
                val crBottomY: Float
                if (isGridLine(pos.line)) {
                    val col = pos.column.coerceIn(0, engine.buffer.lineLength(pos.line))
                    val base = lineTopPx(pos.line) - sY + (refBaselinePx - gridRefBaseline)
                    caretLeft = textX + col * charW
                    crBottomY = base + gridRefCursorBottom
                } else {
                    val layout = layoutFor(pos.line) ?: return
                    val cr = layout.getCursorRect(pos.column.coerceIn(0, engine.buffer.lineLength(pos.line)))
                    val base = lineTopPx(pos.line) - sY + (refBaselinePx - layout.firstBaseline)
                    caretLeft = textX + cr.left
                    crBottomY = base + cr.bottom
                }
                // 按 kind 取预建泪滴（尖端在原点、body 朝外下方悬挂：光标↓、起点↙、终点↘），translate 到光标底后 drawPath。
                val path = when (kind) {
                    HandleKind.Caret -> caretTeardrop
                    HandleKind.SelectionStart -> startTeardrop
                    HandleKind.SelectionEnd -> endTeardrop
                }
                translate(caretLeft, crBottomY) { drawPath(path, colors.handle) }
            }
            if (s == 1f) {
                if (!sel.isEmpty) {
                    if (selectionHandlesVisible()) {
                        drawHandle(HandleKind.SelectionStart, sel.start)
                        drawHandle(HandleKind.SelectionEnd, sel.end)
                    }
                } else if (caretHandleVisible()) {
                    drawHandle(HandleKind.Caret, sel.start)
                }
            }
        }

        // gutter 不透明条 + 行号的横向变换分两种模式：
        //  - 固定于屏幕（PinnedToScreen）：钉左，hTranslate=0，不随缩放焦点横移；正文/光标/手柄画完后铺、盖住横滚进来的正文。
        //  - 固定于行（PinnedToLine）：gutter 属该行坐标系，须与正文用**同一**横向平移 hTranslateText——否则四向缩放横移时
        //    正文按 screenX=s·px+hTranslateText 走、gutter 却按 s·px+0，行号/底色条与所属正文行横向脱节（错位 hTranslateText）。
        //    静止与换行下 hTranslateText=0，退化为原「钉 0」，故此错位只在新的四向缩放横移时可见。
        // 纵向两模式一致（vTranslate）→ 数字与各行对齐、任意横滚下清晰。缩放预览下条高覆盖整条可见带 [preTop, preBottom]。
        val gutterHTranslate = if (pinnedToScreen) 0f else hTranslateText
        withTransform({ translate(gutterHTranslate, vTranslate); scale(s, s, Offset.Zero) }) {
            drawRect(colors.gutterBackground, topLeft = Offset(-gutterScroll, preTop), size = Size(gutterWidthPx, preBottom - preTop))
            for ((line, top) in deferredNumbers) {
                val num = numberLayoutCache.getOrPut(line) { textMeasurer.measure((line + 1).toString(), numberStyle) }
                val numTop = top + (refBaselinePx - num.firstBaseline)
                drawText(
                    num,
                    color = colors.gutterForeground,
                    topLeft = Offset(gutterWidthPx - padXPx - num.size.width - gutterScroll, numTop)
                )
            }
        }
    }
}

/**
 * 只画光标线的独立图层，叠在 [EditorCanvas] 之上。光标位置随滚动 / 光标移动在 draw 阶段实时计算（仅这根细线
 * 重绘）；闪烁 [caretVisible]（= blink）在 graphicsLayer 的 alpha 里读取——每 500ms 翻转只更新该层 alpha
 * （layer 属性 / 合成），不重放正文画布整块的 drawText / gutter / 行号。无选择时才画（有选择时画布画选区、不画光标）。
 * 坐标算法与移除前 EditorCanvas 内的光标段完全一致。
 *
 * 双指缩放预览（[previewScale] != 1f）：本层不参与正文的缩放变换，若照旧画光标会与缩放中的正文脱节，故预览期
 * 直接不画；松手提交后按新布局重新出现，位置正确。
 */
@Composable
fun CursorOverlay(
    engine: EditorEngine,
    colors: EditorColors,
    caretVisible: () -> Boolean,
    scrollX: () -> Float,
    scrollY: () -> Float,
    lineTopPx: (Int) -> Float,
    refBaselinePx: Float,
    layoutFor: (Int) -> TextLayoutResult?,
    charW: Float,
    isGridLine: (Int) -> Boolean,
    gridRefBaseline: Float,
    gridRefCursorTop: Float,
    gridRefCursorBottom: Float,
    gutterWidthPx: Float,
    padXPx: Float,
    previewScale: () -> Float = { 1f },
    modifier: Modifier = Modifier,
) {
    Canvas(modifier.graphicsLayer { alpha = if (caretVisible()) 1f else 0f }) {
        if (previewScale() != 1f) return@Canvas // 缩放预览期不画光标：正文在缩放、光标会脱节
        val sel = engine.selection
        if (!sel.isEmpty) return@Canvas // 有选择时不画光标：画布已画选区覆盖层
        val sX = scrollX()
        val sY = scrollY()
        val textX = gutterWidthPx + padXPx - sX
        val cLine = sel.start.line
        val col = sel.start.column.coerceIn(0, engine.buffer.lineLength(cLine))
        if (isGridLine(cLine)) {
            val textTop = lineTopPx(cLine) - sY + (refBaselinePx - gridRefBaseline)
            val x = textX + col * charW
            drawLine(
                colors.cursor,
                Offset(x, textTop + gridRefCursorTop + 1f),
                Offset(x, textTop + gridRefCursorBottom - 1f),
                strokeWidth = 2.5f
            )
        } else {
            val layout = layoutFor(cLine) ?: return@Canvas
            val textTop = lineTopPx(cLine) - sY + (refBaselinePx - layout.firstBaseline)
            val cr = layout.getCursorRect(col)
            drawLine(
                colors.cursor,
                Offset(textX + cr.left, textTop + cr.top + 1f),
                Offset(textX + cr.left, textTop + cr.bottom - 1f),
                strokeWidth = 2.5f
            )
        }
    }
}

// 放大镜（圆角胶囊）几何常量：固定物理尺寸（dp），内容按 [MAGNIFIER_SCALE] 放大。真机可再调。
private val MAGNIFIER_WIDTH = 132.dp   // 胶囊宽
private val MAGNIFIER_HEIGHT = 82.dp   // 胶囊高（角半径 = 高/2 → 两端半圆）
private val MAGNIFIER_GAP = 22.dp      // 胶囊底与光标顶的间距（浮在其上、指尖不遮）
private val MAGNIFIER_MARGIN = 8.dp    // 窗口边缘留白（胶囊贴边钳住不出界，含上浮到窗口顶时）
private val MAGNIFIER_POPUP_PAD = 30.dp // 胶囊四周留白：须 ≥ 悬浮投影的模糊+下沉 falloff，否则淡入淡出时投影软边被最外 alpha 层的离屏缓冲硬裁
private val MAGNIFIER_BORDER = 1.dp
private const val MAGNIFIER_SCALE = 1.4f

// 悬浮投影（真·dropShadow，画在玻璃层之下）：比旧的画在玻璃内的小阴影更大更柔更沉 → 放大镜浮得更高、更像悬空水珠。
private val MAGNIFIER_DROP_RADIUS = 14.dp
private val MAGNIFIER_DROP_DY = 5.dp
private const val MAGNIFIER_DROP_ALPHA = 0.30f

// 液态玻璃边（折射+色散，无模糊）参数——见 [magnifierGlassRenderEffect]。Android 13+ 与桌面(skiko) 生效，可再调。
private val MAGNIFIER_REFRACTION_HEIGHT = 16.dp // 折射带自胶囊边缘向内的深度
private val MAGNIFIER_REFRACTION_AMOUNT = 10.dp // 边缘最大折射位移
private const val MAGNIFIER_DISPERSION = 0.15f  // 色散强度（0=无、越大彩边越明显）
private const val MAGNIFIER_DEPTH_EFFECT = 1f    // 折射方向向「从中心的径向」混合的强度：0=纯边缘法线(平)、1=域向(鼓起玻璃感)。


/** 当帧放大镜几何：光标屏幕位置、连续参考点、胶囊矩形——[MagnifierOverlay] 的 graphicsLayer（算玻璃边矩形）与 draw 共用，保证对齐。 */
private class LoupeGeom(
    val cLine: Int,
    val col: Int,
    val caretX: Float,
    val caretTopY: Float,
    val caretBotY: Float,
    val refX: Float,
    val refY: Float,
    val cx: Float,
    val cyc: Float,
    val rect: Rect,
    val radius: Float,
)

/**
 * 移动光标 / 选区端点手柄时的放大镜层，叠在 [EditorCanvas]/[CursorOverlay] 之上（[active] 为真才画，松手即空）。
 * 圆角胶囊内**重绘**光标附近数行 + 光标线，绕光标点按 [MAGNIFIER_SCALE] 放大——非位图快照（会糊；本项目缩放亦
 * 否决位图放大）。坐标系与 [EditorCanvas] 完全一致（textX = gutter+pad−sX、drawText(layoutFor(line))），只是外面套一层
 * 「绕参考点缩放 + 平移到胶囊中心」的 withTransform 并裁到胶囊形。
 *
 * **两轴都用连续手指位置（[dragPos]）而非离散光标**：光标只落在字符/行边界、按格跳，若胶囊/内容以 caretX/caretMidY 为
 * 参考会跟着跳；改以逐帧手指 (x,y) 为参考（y 已含 grabDy ≈ 光标中心）→ 胶囊与放大内容两轴平滑跟手，光标线仍画在离散
 * caretX/caretY、只在镜内按字符/行吸附。放大目标行 = [EditorEngine.caret]（选区端点拖拽即活动端 head），逐帧在 draw 读 →
 * 零重组。超长网格行无整行 layout，测量光标附近一段列窗口绘制。
 *
 * 忠实镜像编辑器的光标/选区：空选区（拖光标手柄）画正文 + 随 [caretVisible]（blink）闪烁的光标；有选区（拖端点）画
 * 正文 + [EditorColors.selection] 选区底色、不画光标（端点即选区高亮边缘）——与 [EditorCanvas]/[CursorOverlay] 一致。
 *
 * 边缘为「液态玻璃」（折射 + 色散，无模糊）+ 一圈内阴影（立体感）。Android 13+ 与桌面(skiko) 有实现，Android <13 退化描边。
 *
 * **宿主在窗口级 [Popup]**（而非编辑器内 Canvas）：胶囊需上浮到编辑器上方的工具栏/状态栏空间（近顶不被手指遮），但编辑器 Box
 * 有 clipToBounds、且本层带 RenderEffect 会渲到与节点等大的离屏缓冲 → 画不出编辑器边界。故改为:一个零尺寸「锚」以 deferred
 * `offset{}` 逐帧移到胶囊「编辑器局部」左上角（layout 阶段跟随、无重组），[Popup] 锚定它 → 映射到窗口坐标、越出编辑器边界。
 * Popup 内容仅「胶囊 + [MAGNIFIER_POPUP_PAD]」大小、定位在手指上方 → 不抢拖拽触摸；玻璃 shader 只作用这一小块、uniform 恒定只建一次。
 */
@Composable
fun MagnifierOverlay(
    engine: EditorEngine,
    colors: EditorColors,
    active: () -> Boolean,
    dragPos: () -> Offset?,
    caretVisible: () -> Boolean,
    viewportWidth: () -> Float,
    contentTopInWindow: () -> Float,
    scrollX: () -> Float,
    scrollY: () -> Float,
    lineTopPx: (Int) -> Float,
    lineHeightPx: Float,
    refBaselinePx: Float,
    layoutFor: (Int) -> TextLayoutResult?,
    textMeasurer: TextMeasurer,
    textStyle: TextStyle,
    numberStyle: TextStyle,
    charW: Float,
    isGridLine: (Int) -> Boolean,
    gridRefBaseline: Float,
    gridRefCursorTop: Float,
    gridRefCursorBottom: Float,
    gutterWidthPx: Float,
    padXPx: Float,
    // 语法高亮状态链（null = 无高亮）：网格长行切片取着色段，与主画布同源。
    highlightCache: HighlightCache? = null,
) {
    // show/hide 快速淡入淡出：alpha 随 active 起落，Popup 内容 graphicsLayer 应用整层透明度。淡入偏快（抓取即现）、淡出稍慢
    // （消失最显突兀，给足过渡）；都仍是「快速」量级，时长可再调。
    val showing = active()
    val alpha = animateFloatAsState(
        targetValue = if (showing) 1f else 0f,
        animationSpec = tween(durationMillis = if (showing) 160 else 260),
        label = "magnifierAlpha",
    )
    // Popup 存在与否：alpha>~0（可见或淡出中）才挂 Popup。用 derivedStateOf 只在阈值翻转时重组（非逐帧）；淡出的渐隐由内容
    // graphicsLayer 的 alpha 完成，alpha≈0 时才撤 Popup。
    val visible by remember { derivedStateOf { alpha.value > 0.001f } }
    // 最后一帧的连续参考点 [x,y]（普通 remember、非快照——写不触发重组/重绘）。松手后 dragPos 置空时用它 → 胶囊留在原位淡出，
    // 而非回落到吸附后的最终光标位置再淡出。NaN 表示尚无有效值（回落到光标）。
    val lastRef = remember { floatArrayOf(Float.NaN, Float.NaN) }
    // 支持液态玻璃边（Android 13+ / 桌面）时靠玻璃 rim 界定边缘、不画硬描边；不支持（Android <13）时才退化画描边。
    val glassSupported = remember { isMagnifierGlassSupported() }
    val density = LocalDensity.current
    // 玻璃 RenderEffect：胶囊在 Popup 内容中固定于 (pad,pad,wl,hl) → uniform 恒定、只建一次；作用图层仅「胶囊+2·pad」大小 → shader 有界省算。
    val glassEffect = remember(density, glassSupported) {
        if (!glassSupported) null else with(density) {
            magnifierGlassRenderEffect(
                left = MAGNIFIER_POPUP_PAD.toPx(),
                top = MAGNIFIER_POPUP_PAD.toPx(),
                width = MAGNIFIER_WIDTH.toPx(),
                height = MAGNIFIER_HEIGHT.toPx(),
                cornerRadius = MAGNIFIER_HEIGHT.toPx() / 2f,
                refractionHeight = MAGNIFIER_REFRACTION_HEIGHT.toPx(),
                refractionAmount = MAGNIFIER_REFRACTION_AMOUNT.toPx(),
                depthEffect = MAGNIFIER_DEPTH_EFFECT,
                chromaticAberration = MAGNIFIER_DISPERSION,
            )
        }
    }

    // 当帧胶囊几何（**编辑器局部坐标**）。offset 锚（定位 Popup）与 Popup 内 Canvas（画内容）都调它 → 同帧同值。dragPos 非空时记
    // lastRef（两处调用同帧同值、幂等）。返回 null（当前行无 layout）时不定位/不画。
    fun computeLoupe(d: Density): LoupeGeom? {
        val sX = scrollX()
        val sY = scrollY()
        val pos = engine.caret
        val cLine = pos.line
        val col = pos.column.coerceIn(0, engine.buffer.lineLength(cLine))
        val textX = gutterWidthPx + padXPx - sX
        val caretX: Float
        val caretTopY: Float
        val caretBotY: Float
        if (isGridLine(cLine)) {
            val base = lineTopPx(cLine) - sY + (refBaselinePx - gridRefBaseline)
            caretX = textX + col * charW
            caretTopY = base + gridRefCursorTop
            caretBotY = base + gridRefCursorBottom
        } else {
            val layout = layoutFor(cLine) ?: return null
            val cr = layout.getCursorRect(col)
            val base = lineTopPx(cLine) - sY + (refBaselinePx - layout.firstBaseline)
            caretX = textX + cr.left
            caretTopY = base + cr.top
            caretBotY = base + cr.bottom
        }
        val caretMidY = (caretTopY + caretBotY) / 2f
        // 两轴参考用连续手指位置（拖拽点：x 为原始手指 x、y 为含 grabDy 的光标中心 y），松手无拖拽点时用 lastRef 定格、再无回落光标。
        val dp = dragPos()
        val refX: Float
        val refY: Float
        if (dp != null) {
            refX = dp.x
            refY = dp.y
            lastRef[0] = refX
            lastRef[1] = refY
        } else {
            refX = if (lastRef[0].isNaN()) caretX else lastRef[0]
            refY = if (lastRef[1].isNaN()) caretMidY else lastRef[1]
        }
        val wl = with(d) { MAGNIFIER_WIDTH.toPx() }
        val hl = with(d) { MAGNIFIER_HEIGHT.toPx() }
        val gap = with(d) { MAGNIFIER_GAP.toPx() }
        val margin = with(d) { MAGNIFIER_MARGIN.toPx() }
        val radius = hl / 2f
        val halfW = wl / 2f
        // 胶囊中心 x 跟随手指、贴左右边缘钳住（按视口宽）；垂直始终浮在光标上方 gap（refY≈光标中心，减半行 → 连续「光标顶」）。
        val cx = refX.coerceIn(margin + halfW, (viewportWidth() - margin - halfW).coerceAtLeast(margin + halfW))
        // 顶部下限允许上浮到窗口顶附近（越过工具栏/状态栏）：胶囊顶在窗口 y ≥ margin ⇒ 编辑器局部 top ≥ margin − 内容顶在窗口 y。
        val minTop = margin - contentTopInWindow()
        val top = (refY - lineHeightPx / 2f - gap - hl).coerceAtLeast(minTop)
        val cyc = top + hl / 2f
        return LoupeGeom(cLine, col, caretX, caretTopY, caretBotY, refX, refY, cx, cyc, Rect(cx - halfW, top, cx + halfW, top + hl), radius)
    }

    // 零尺寸锚：deferred offset 到胶囊「编辑器局部」左上角（layout 阶段逐帧跟随、无重组；闲时 alpha≈0 直接归零、不读 caret/scroll）。
    // Popup 锚定它 → 映射到窗口坐标、越出编辑器 clipToBounds 的边界，可浮到工具栏/状态栏上。
    Box(
        Modifier.offset {
            if (alpha.value <= 0.001f) IntOffset.Zero
            else computeLoupe(this)?.let { IntOffset(it.rect.left.roundToInt(), it.rect.top.roundToInt()) } ?: IntOffset.Zero
        }
    ) {
        if (visible) {
            val padPx = with(density) { MAGNIFIER_POPUP_PAD.roundToPx() }
            Popup(
                alignment = Alignment.TopStart,
                offset = IntOffset(-padPx, -padPx), // 胶囊在 Popup 内容里位于 (pad,pad)，故内容左上 = 锚(胶囊左上) − pad
                properties = PopupProperties(focusable = false, clippingEnabled = false),
            ) {
                // 内容分三层，整组套【一个】alpha 层统一淡入淡出（放在最外 wrapping Box、缓冲含四周 pad → 悬浮投影软边不被离屏缓冲
                // 边界硬裁；也避免每层各自半透叠加的脏合成）：① 底层悬浮投影（真·dropShadow、在玻璃之下）；② 玻璃 Canvas（放大内容+折射/色散）；
                // ③ 顶层玻璃质感 overlay（内阴影收边 + 凸面薄顶光/底阴 + 顶缘光，均在玻璃之上、不被折射）——三层叠出"浮在屏上的水珠"立体感。
                Box(
                    Modifier
                        .size(MAGNIFIER_WIDTH + MAGNIFIER_POPUP_PAD * 2, MAGNIFIER_HEIGHT + MAGNIFIER_POPUP_PAD * 2)
                        .graphicsLayer { this.alpha = alpha.value },
                ) {
                    // ① 悬浮投影：胶囊形 dropShadow，画在玻璃层之下（首个子节点）。
                    Box(
                        Modifier
                            .offset(MAGNIFIER_POPUP_PAD, MAGNIFIER_POPUP_PAD)
                            .size(MAGNIFIER_WIDTH, MAGNIFIER_HEIGHT)
                            .dropShadow(
                                shape = RoundedCornerShape(percent = 50),
                                shadow = Shadow(
                                    radius = MAGNIFIER_DROP_RADIUS,
                                    spread = 0.dp,
                                    color = Color.Black.copy(alpha = MAGNIFIER_DROP_ALPHA),
                                    offset = DpOffset(0.dp, MAGNIFIER_DROP_DY),
                                ),
                            ),
                    )
                    Canvas(
                        Modifier
                            .size(MAGNIFIER_WIDTH + MAGNIFIER_POPUP_PAD * 2, MAGNIFIER_HEIGHT + MAGNIFIER_POPUP_PAD * 2)
                            .graphicsLayer {
                                renderEffect = glassEffect // 恒定：胶囊在图层内固定位置，uniform 不变、只建一次；alpha 由最外 wrapping 层统一施加
                            }
                    ) {
                        val g = computeLoupe(this) ?: return@Canvas
                        val m = MAGNIFIER_SCALE
                        val pad = MAGNIFIER_POPUP_PAD.toPx()
                        val wl = MAGNIFIER_WIDTH.toPx()
                        val hl = MAGNIFIER_HEIGHT.toPx()
                        val radius = g.radius
                        val sY = scrollY()
                        val textX = gutterWidthPx + padXPx - scrollX()
                        val sel = engine.selection // 有选区（拖端点）→ 画选区底色、不画光标；空选区（拖光标）→ 画随 blink 的光标
                        val loupeRect = Rect(pad, pad, pad + wl, pad + hl) // Popup 内容局部坐标（胶囊固定于此、玻璃 uniform 与之对齐）

                        // 悬浮投影不在此画：已移到玻璃层【之下】的真·dropShadow（见 Popup 内容第 ① 层），更大更柔、浮得更高。
                        val clip = Path().apply { addRoundRect(RoundRect(loupeRect, CornerRadius(radius, radius))) }
                        clipPath(clip) {
                            drawRect(colors.background, topLeft = loupeRect.topLeft, size = loupeRect.size)
                            // 放大变换：编辑器局部内容点 (ex,ey) → Popup 内胶囊中心 (pad+wl/2, pad+hl/2)，绕连续参考点 (refX,refY) 放大 m。
                            withTransform({
                                translate((pad + wl / 2f) - m * g.refX, (pad + hl / 2f) - m * g.refY)
                                scale(m, m, Offset.Zero)
                            }) {
                                val lineCount = engine.buffer.lineCount
                                val first = (g.cLine - 2).coerceAtLeast(0)
                                val last = (g.cLine + 2).coerceAtMost(lineCount - 1)
                                var ln = first
                                while (ln <= last) {
                                    val lineTop = lineTopPx(ln) - sY
                                    // 选区底色（在正文下）：与 EditorCanvas 同式——本行落在 [sel.start.line, sel.end.line] 内即画首/末列间的高亮，
                                    // 非末行再补一小段行尾换行位。网格行走等宽矩形、其余走 layout 的 range path。
                                    val inSel = !sel.isEmpty && ln >= sel.start.line && ln <= sel.end.line
                                    if (isGridLine(ln)) {
                                        val textTop = lineTop + (refBaselinePx - gridRefBaseline)
                                        val len = engine.buffer.lineLength(ln)
                                        if (inSel) {
                                            val cS = if (ln == sel.start.line) sel.start.column else 0
                                            val cE = if (ln == sel.end.line) sel.end.column else len
                                            if (cE > cS) drawRect(
                                                colors.selection,
                                                topLeft = Offset(textX + cS * charW, lineTop),
                                                size = Size((cE - cS) * charW, lineHeightPx)
                                            )
                                            if (ln != sel.end.line) drawRect(
                                                colors.selection,
                                                topLeft = Offset(textX + len * charW, lineTop),
                                                size = Size(lineHeightPx * 0.4f, lineHeightPx)
                                            )
                                        }
                                        val c0 = (g.col - 24).coerceIn(0, len)
                                        val c1 = (g.col + 24).coerceIn(0, len)
                                        if (c1 > c0) {
                                            // 与主画布同源：切片带语法着色（放大镜逐帧重测，单行 ≤ 上限的重扫是微秒级）。
                                            val sl =
                                                measureGridSlice(engine, textMeasurer, textStyle, colors.syntax, highlightCache, ln, c0, c1)
                                            drawText(sl, topLeft = Offset(textX + c0 * charW, textTop))
                                        }
                                    } else {
                                        val layout = layoutFor(ln)
                                        if (layout != null) {
                                            val textTop = lineTop + (refBaselinePx - layout.firstBaseline)
                                            if (inSel) {
                                                val len = engine.buffer.lineLength(ln)
                                                val cS = if (ln == sel.start.line) sel.start.column else 0
                                                val cE = if (ln == sel.end.line) sel.end.column else len
                                                if (cE > cS) {
                                                    val path = layout.getPathForRange(cS, cE)
                                                    path.translate(Offset(textX, lineTop)) // 选区底色锚固定行栅格（同主画布），不随基线平移
                                                    drawPath(path, colors.selection)
                                                }
                                                if (ln != sel.end.line) {
                                                    val cr = layout.getCursorRect(len)
                                                    drawRect(
                                                        colors.selection,
                                                        topLeft = Offset(textX + cr.left, lineTop),
                                                        size = Size(lineHeightPx * 0.4f, lineHeightPx)
                                                    )
                                                }
                                            }
                                            // 不传 color：镜内正文沿用 layout 自带的语法高亮色，与主画布一致。
                                            drawText(layout, topLeft = Offset(textX, textTop))
                                        }
                                    }
                                    ln++
                                }
                                // 行号 gutter（编辑器局部坐标 = 主画布屏幕坐标，随内容一同放大）：正文之后铺不透明底色条 [0, gutterWidth]、
                                // 行号右对齐、基线对齐正文基线——与主画布同式。画在正文之后 ⇒ 横滚时盖住滚进 gutter 区的正文。放大镜是绕光标
                                // 的窗口，仅光标靠行首时 gutter 落入镜窗、自然可见（滚到行中时 gutter 在窗外、不画，符合「放大邻域」语义）。
                                run {
                                    val gTop = lineTopPx(first) - sY
                                    val gBot = lineTopPx(last) - sY + lineHeightPx
                                    drawRect(colors.gutterBackground, topLeft = Offset(0f, gTop), size = Size(gutterWidthPx, gBot - gTop))
                                    var gln = first
                                    while (gln <= last) {
                                        val lt = lineTopPx(gln) - sY
                                        val num = textMeasurer.measure((gln + 1).toString(), numberStyle)
                                        val numTop = lt + (refBaselinePx - num.firstBaseline)
                                        drawText(
                                            num,
                                            color = colors.gutterForeground,
                                            topLeft = Offset(gutterWidthPx - padXPx - num.size.width, numTop)
                                        )
                                        gln++
                                    }
                                }
                                // 光标线：仅空选区（拖光标手柄）时画，且随 blink（caretVisible）闪烁；有选区（拖端点）时不画、端点由选区边缘体现。
                                if (sel.isEmpty && caretVisible()) {
                                    drawLine(
                                        colors.cursor,
                                        Offset(g.caretX, g.caretTopY),
                                        Offset(g.caretX, g.caretBotY),
                                        strokeWidth = 1.6f
                                    )
                                }
                            }
                            // 内阴影不在此画：已移到玻璃层【之上】的 innerShadow overlay（不被折射、不与玻璃折射 rim 打架）——见下方 Popup 内容第二层。
                        }
                        // 边框：仅无液态玻璃边的平台画（玻璃 rim 已界定边缘；有玻璃时再画硬描边会与折射 rim 打架）。
                        if (!glassSupported) {
                            drawRoundRect(
                                color = colors.gutterForeground.copy(alpha = 0.35f),
                                topLeft = loupeRect.topLeft,
                                size = loupeRect.size,
                                cornerRadius = CornerRadius(radius, radius),
                                style = Stroke(width = MAGNIFIER_BORDER.toPx()),
                            )
                        }
                    }
                    // ③ 玻璃质感 overlay：只留一层 innerShadow 收边。玻璃的"鼓起"改由折射 shader 的 depthEffect 提供（见 magnifierGlassRenderEffect）——
                    // overlay 不再画渐变/rim（那些盖在正文上的手绘光看着不自然，已撤）。
                    Box(
                        Modifier
                            .offset(MAGNIFIER_POPUP_PAD, MAGNIFIER_POPUP_PAD)
                            .size(MAGNIFIER_WIDTH, MAGNIFIER_HEIGHT)
                            .innerShadow(
                                shape = RoundedCornerShape(percent = 50),
                                shadow = Shadow(
                                    radius = 14.dp,
                                    spread = 0.dp,
                                    color = Color.Black.copy(alpha = 0.25f),
                                    offset = DpOffset(0.dp, 1.dp)
                                ),
                            ),
                    )
                }
            }
        }
    }
}
