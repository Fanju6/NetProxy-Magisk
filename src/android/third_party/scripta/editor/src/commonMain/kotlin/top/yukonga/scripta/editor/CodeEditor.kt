package top.yukonga.scripta.editor

import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.focusable
import androidx.compose.foundation.gestures.awaitEachGesture
import androidx.compose.foundation.gestures.awaitFirstDown
import androidx.compose.foundation.gestures.awaitTouchSlopOrCancellation
import androidx.compose.foundation.gestures.calculateCentroid
import androidx.compose.foundation.gestures.calculateZoom
import androidx.compose.foundation.gestures.detectDragGesturesAfterLongPress
import androidx.compose.foundation.gestures.drag
import androidx.compose.foundation.gestures.rememberScrollable2DState
import androidx.compose.foundation.gestures.scrollable2D
import androidx.compose.foundation.gestures.waitForUpOrCancellation
import androidx.compose.foundation.interaction.DragInteraction
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.interaction.collectIsFocusedAsState
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.WindowInsetsSides
import androidx.compose.foundation.layout.captionBar
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.ime
import androidx.compose.foundation.layout.navigationBars
import androidx.compose.foundation.layout.only
import androidx.compose.foundation.layout.union
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.overscroll
import androidx.compose.foundation.rememberOverscrollEffect
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.runtime.snapshotFlow
import androidx.compose.runtime.withFrameNanos
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clipToBounds
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.hapticfeedback.HapticFeedbackType
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.isShiftPressed
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.onKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.input.pointer.PointerEventType
import androidx.compose.ui.input.pointer.PointerIcon
import androidx.compose.ui.input.pointer.PointerType
import androidx.compose.ui.input.pointer.isPrimaryPressed
import androidx.compose.ui.input.pointer.isSecondaryPressed
import androidx.compose.ui.input.pointer.pointerHoverIcon
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.onGloballyPositioned
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.layout.positionInWindow
import androidx.compose.ui.platform.LocalClipboard
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalHapticFeedback
import androidx.compose.ui.text.TextLayoutResult
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.rememberTextMeasurer
import androidx.compose.ui.text.style.LineHeightStyle
import androidx.compose.ui.text.style.TextMotion
import androidx.compose.ui.unit.Constraints
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import kotlinx.coroutines.delay
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.collectLatest
import top.yukonga.scripta.editor.completion.CompletionPopup
import top.yukonga.scripta.editor.completion.CompletionProvider
import top.yukonga.scripta.editor.completion.CompletionRequest
import top.yukonga.scripta.editor.completion.CompletionResult
import top.yukonga.scripta.editor.find.FindReplaceBar
import top.yukonga.scripta.editor.find.GotoLineBar
import top.yukonga.scripta.editor.highlight.HighlightCache
import top.yukonga.scripta.editor.highlight.HighlightSpan
import top.yukonga.scripta.editor.highlight.SyntaxHighlighter
import top.yukonga.scripta.editor.highlight.YamlHighlighter
import top.yukonga.scripta.editor.highlight.highlightedText
import top.yukonga.scripta.editor.input.EditorKeyCommand
import top.yukonga.scripta.editor.input.editorRightEdgeGestureExclusion
import top.yukonga.scripta.editor.input.editorTextInput
import top.yukonga.scripta.editor.input.insertTypedCharacter
import top.yukonga.scripta.editor.input.resolveEditorKeyCommand
import top.yukonga.scripta.editor.menu.EditorClipboardActions
import top.yukonga.scripta.editor.menu.EditorContextAction
import top.yukonga.scripta.editor.menu.EditorContextMenu
import top.yukonga.scripta.editor.menu.SelectionActionToolbar
import top.yukonga.scripta.editor.render.CursorOverlay
import top.yukonga.scripta.editor.render.EditorCanvas
import top.yukonga.scripta.editor.render.EditorGeometry
import top.yukonga.scripta.editor.render.HandleKind
import top.yukonga.scripta.editor.render.MagnifierOverlay
import top.yukonga.scripta.editor.render.ScrollbarMath
import top.yukonga.scripta.editor.render.ScrollbarOverlay
import top.yukonga.scripta.editor.render.VisualRowIndex
import top.yukonga.scripta.editor.render.ZoomMath
import top.yukonga.scripta.editor.text.BracketMatcher
import top.yukonga.scripta.editor.text.TextPosition
import top.yukonga.scripta.editor.text.TextRange
import kotlin.math.abs
import kotlin.time.Duration.Companion.milliseconds
import kotlin.time.TimeSource
import androidx.compose.ui.input.pointer.isShiftPressed as isKeyboardShiftPressed

/** 超过此字符数的行走「网格」快路：不整行 shaping，只按可见列窗口等宽算术定位/绘制（M2，仅不换行）。 */
private const val LONG_LINE_THRESHOLD = 2000

/**
 * 双指缩放提交的最小字号变化阈值（sp）：净变化小于此值视作未真正缩放、不重排。只是滤掉双指微动噪声（避免无谓重排
 * 与非整字号漂移）；不吸附提交值——提交字号恒为精确的 `起始字号 × 连续系数`，与预览末帧完全连续、无提交跳变。
 * （旧版曾量化到 0.5sp 档以压制「手势中逐档重排」；现只在松手重排一次、无此性能压力，量化已无必要且会引入提交吸附跳变。）
 */
private const val ZOOM_MIN_CHANGE_SP = 0.1f

/** 双指缩放字号下限 / 上限（sp）。手势中连续缩放系数被夹到使字号落在此区间，提交后字号亦钳于此。 */
private const val ZOOM_MIN_SP = 8f
private const val ZOOM_MAX_SP = 40f

/**
 * 布局基准字号与派生量比例：行号字号 = 正文 × [NUMBER_FONT_SCALE]（默认 14sp→13sp）；横向内边距
 * padX = 正文 / [LAYOUT_BASE_FONT_SP] × [PAD_X_BASE_DP]（默认 14sp→8dp）。**关键**：用「比例」而非「常量偏移 / 固定 dp」，
 * 使 gutter 宽、内边距随字号线性缩放，整套布局在缩放下「自相似」——双指缩放预览（整体等比缩放）与松手重排（按新字号
 * 重新布局）产生一致几何，行号/正文在提交时不跳变、无重绘；不换行提交横向亦简化为精确的 sX'=k·sX。
 */
private const val LAYOUT_BASE_FONT_SP = 14f
private const val NUMBER_FONT_SCALE = 13f / 14f
private const val PAD_X_BASE_DP = 8f

/**
 * 底部可滚过文末的留白（视口高比例）：允许主动滚动把末行上移、下方留空，不把末行钉死底边。
 * 双上界避免历史坑：留白只并进「主动滚动/渲染/命中」用的 maxScrollY；所有**自动 settle**（IME 收键盘 re-clamp、
 * keep-in-view、缩放提交的 re-clamp）走不含留白的 maxScrollYSettle（自然底、末行贴底）。否则底部弹键盘把 scrollY
 * 推大、收键盘后 re-clamp 夹到含留白上界，会在末行下方残留一块空白。0.2 = 末行最多上移到距底 1/5 视口。
 */
private const val BOTTOM_SCROLL_PAD_FRACTION = 0.2f

private const val COMPLETION_DEBOUNCE_MS = 120L

/** 补全请求只订阅文档、光标和编辑状态，不把滚动等帧率状态带入协程。 */
private data class CompletionTrigger(
    val documentVersion: Int,
    val caret: TextPosition,
    val selection: TextRange,
    val composing: Boolean,
    val requestToken: Int,
    val findVisible: Boolean,
    val gotoVisible: Boolean,
)

/** 右侧可滚过行尾的留白（视口宽比例）：滚满时最长行尾不贴屏幕右缘，行尾编辑有呼吸空间——否则只余
 *  一个 padX（~8dp）的贴边感。取比底部小的比例：横向空间更稀缺，10% ≈ 四五个字符宽已够。无需
 *  Settle 双上界（那是收键盘视口高复原的专属坑，视口宽没有对应的自动复原场景）。 */
private const val RIGHT_SCROLL_PAD_FRACTION = 0.1f

// 滚动条：thumb 最小高（超长文档比例高会缩成几像素、无法抓取）/ 右缘可抓热区宽 / 命中的纵向余量。
private val SCROLLBAR_MIN_THUMB = 48.dp
private val SCROLLBAR_HOT_ZONE = 24.dp
private val SCROLLBAR_GRAB_SLACK = 8.dp

// 静止多久开始淡出 / 淡入淡出时长 / 「滚动刚停」静默期（fling 进行中 thumb 不可抓——沿右缘起手的
// 下一次 flick 会被绝对映射吃成半个文档的跳转，且 fling 循环与拖拽循环会同帧竞写 scrollY）。
private const val SCROLLBAR_IDLE_MS = 1200
private const val SCROLLBAR_QUIET_GRAB_MS = 150

/** 滚动条显示状态机在回调/看门协程间共享的非 state 部件（全部 UI 线程）：活动时间戳 + 低频沿镜像。
 *  时间戳每滚动帧都写，用普通字段而非快照 state（本文件的每帧零装箱纪律，见 scrollY 声明处）。 */
private class ScrollbarClock {
    var last: TimeSource.Monotonic.ValueTimeMark = TimeSource.Monotonic.markNow()
    var shown = false
    var fading = false
    var hovering = false
}

/**
 * layoutFor 逐行缓存条目。[validatedVersion] 记「上次确认与文档一致时的 buffer.version」：命中且版本相等即
 * 说明此后无任何编辑、本行文本必与 [content] 相同，可零分配零比较直接复用 [layout]（稳态滚动/闪烁/拖选帧全
 * 走这里）；版本变了才回退按 [content] + [spans] 比对——编辑他行不改本行文本，但可能经跨行状态（块标量）
 * 改本行着色，故着色段也参与校验；两者都相符才复用并刷新版本戳。
 */
private class CachedLayout(
    var validatedVersion: Int,
    val content: String,
    val spans: List<HighlightSpan>,
    val layout: TextLayoutResult,
)

/**
 * isNarrowLine 逐行缓存条目（与 [CachedLayout] 同法失效，但只存「长度+hash 指纹」而非整行内容——网格路专治
 * 超长行，把整串钉在缓存里会为单条巨行长期占用兆字节）。[narrow] = 该行是否全为可打印 ASCII（等宽半角），
 * 即能否走网格快路；含 CJK/全角/emoji 等非等宽字形时为 false，令该长行退回整行 shaping。
 */
private class NarrowFlag(var validatedVersion: Int, val length: Int, val hash: Int, val narrow: Boolean)

/** 网格快路要求的「窄行」判定：全部为可打印 ASCII（U+0020..U+007E，等宽半角）。含任何 CJK/全角/emoji/制表符
 *  等宽度≠charW 或竖直度量异于 ASCII 的字符即返回 false——保守但安全，误判只是让该长行退回 shaping、不影响正确性。 */
private fun isNarrowGridText(s: String): Boolean = s.all { it.code in 0x20..0x7E }

/**
 * 虚拟化代码编辑器入口。自绘 + 视口虚拟化 + 自管 IME（Android），不使用 BasicTextField。
 *
 * **底部系统栏（导航栏 / 键盘）由编辑器内部消费**，宿主不要再对编辑器加 `imePadding()`/导航栏 padding
 * （否则重复让位、且底色无法铺进小白条区）——宿主只需给它 `weight`/尺寸即可。内部据是否显示符号条决定谁
 * 让开系统栏：显示时符号条背景铺到屏幕边缘、键抬到栏上；隐藏时文本区让开系统栏、编辑器底色铺到边缘。
 * 这样可用视口高度始终反映键盘弹出后的可见高度，光标随动不会滚到键盘下面。
 *
 * [symbols] 非空且非只读时显示符号快捷栏，文本视口会为其让位。传 `emptyList()` 关闭；只读时不显示。
 * [symbolBarPosition] 可将符号栏改为停靠在编辑器右侧。[bottomBar] 用于注入自定义底部操作栏，
 * 编辑器会统一处理键盘与系统导航栏安全区。
 */
@Composable
fun CodeEditor(
    controller: CodeEditorController,
    modifier: Modifier = Modifier,
    language: EditorLanguage = EditorLanguage.PlainText,
    colors: EditorColors = EditorColors.Default,
    readOnly: Boolean = false,
    softWrap: Boolean = false,
    lineNumberMode: LineNumberMode = LineNumberMode.PinnedToScreen,
    symbols: List<EditorSymbol> = DefaultEditorSymbols,
    symbolBarPosition: SymbolBarPosition = SymbolBarPosition.Bottom,
    bottomBar: (@Composable (EditorColors) -> Unit)? = null,
    /** 键入智能：括号/引号自动配对、跳过闭合、成对退格。改变输入行为本身，故给宿主关闭权
     *  （匹配高亮不受此控——纯视觉、无输入副作用）。 */
    autoClosePairs: Boolean = true,
    /** 代码补全提供方；null 时不启用自动或显式补全。 */
    completionProvider: CompletionProvider? = null,
    /**
     * 语法高亮插件；null 时按 [language] 选内置插件（PlainText = 不高亮）。自定义插件优先于内置。
     * 实例须跨重组稳定（`remember` 住再传入）——它是行状态链缓存与 layout 缓存的 key，
     * 每次重组换新实例会把两级缓存整个冲掉、逐帧全量重算。
     */
    highlighter: SyntaxHighlighter? = null,
) {
    // 初始内容在 rememberCodeEditorController(initialText) 工厂里播种（构造即播种：无空文档首帧、
    // 无重组重播风险）；换文档走 controller.setDocument。本 composable 不再有文本入参。
    val engine = controller.engine
    val findSession = controller.find
    val gotoSession = controller.gotoLine
    engine.autoClosePairs = autoClosePairs // 键入路径读引擎字段（非 state），组合期同步参数即可

    // 匹配括号：光标邻接 ()[]{} 时点亮该括号与配对方。derivedState 依赖 caret（选区 state 派生）与
    // 文档版本（编辑改配对），扫描有界（BracketMatcher 预算）；draw 阶段读值——光标移动只重绘不重组。
    val bracketMatchState = remember(engine) {
        derivedStateOf {
            engine.buffer.version
            BracketMatcher.findMatch(engine.buffer, engine.caret)
        }
    }

    // 查找结果重算：可见性 / 查询串 / 三个开关 / 文档版本任一变化即重算（snapshotFlow 只订阅这些读取，
    // 不牵动本可组合重组）。列表做结构比较，去掉纯粹的重复快照。
    LaunchedEffect(engine) {
        snapshotFlow {
            listOf(
                findSession.visible, findSession.query,
                findSession.caseSensitive, findSession.wholeWord, findSession.useRegex,
                engine.buffer.version,
            )
        }.collectLatest { findSession.refresh() }
    }

    var completionResult by remember(engine, completionProvider) {
        mutableStateOf<CompletionResult?>(null)
    }
    var completionSelectedIndex by remember(engine, completionProvider) { mutableIntStateOf(0) }
    var previousCompletionRequestToken by remember(controller) {
        mutableIntStateOf(controller.completionRequestToken)
    }

    // 自动补全对连续输入做短防抖；显式请求立即执行。结果返回时再次核对版本和光标，避免异步旧结果
    // 覆盖新文档。提供方异常只关闭候选，不影响编辑器输入。
    LaunchedEffect(engine, completionProvider, readOnly) {
        snapshotFlow {
            CompletionTrigger(
                documentVersion = engine.buffer.version,
                caret = engine.caret,
                selection = engine.selection,
                composing = engine.composing != null,
                requestToken = controller.completionRequestToken,
                findVisible = findSession.visible,
                gotoVisible = gotoSession.visible,
            )
        }.collectLatest { trigger ->
            val explicit = trigger.requestToken != previousCompletionRequestToken
            previousCompletionRequestToken = trigger.requestToken
            completionResult = null
            completionSelectedIndex = 0

            val provider = completionProvider ?: return@collectLatest
            if (readOnly || trigger.composing || !trigger.selection.isEmpty ||
                trigger.findVisible || trigger.gotoVisible
            ) {
                return@collectLatest
            }
            if (!explicit) delay(COMPLETION_DEBOUNCE_MS)

            val text = engine.getText()
            val result = try {
                provider.complete(
                    CompletionRequest(
                        text = text,
                        caret = trigger.caret,
                        explicit = explicit,
                    ),
                )
            } catch (error: CancellationException) {
                throw error
            } catch (_: Exception) {
                null
            }
            if (engine.buffer.version == trigger.documentVersion && engine.caret == trigger.caret) {
                completionResult = result?.takeIf { it.items.isNotEmpty() }
            }
        }
    }

    val acceptCompletion: (Int) -> Unit = { index ->
        val result = completionResult
        val item = result?.items?.getOrNull(index)
        if (!readOnly && result != null && item != null) {
            controller.replaceRange(result.replaceRange.start, result.replaceRange.end, item.insertText)
        }
        completionResult = null
    }

    val moveCompletionSelection: (Int) -> Unit = { delta ->
        val count = completionResult?.items?.size ?: 0
        if (count > 0) {
            completionSelectedIndex = (completionSelectedIndex + delta + count) % count
        }
    }

    val density = LocalDensity.current
    val measurer = rememberTextMeasurer()
    val focusRequester = remember { FocusRequester() }
    val interaction = remember { MutableInteractionSource() }
    // 编辑器（focusable 节点）是否持有焦点：查找框等部件夺焦时为 false。光标/光标手柄只在持焦时绘制——
    // 焦点唯一由 Compose 保证，这里只是让绘制尊重它（否则查找框和正文会同时闪两个光标）。
    // **仅在 draw 阶段的 lambda 里读**：焦点切换只重绘光标层，不牵动本可组合重组。
    val editorFocused = interaction.collectIsFocusedAsState()
    // 新 Clipboard 的收发是 suspend，需一个组合级作用域来跑复制/剪切/粘贴。
    val clipboard = LocalClipboard.current
    val clipboardScope = rememberCoroutineScope()
    val haptic = LocalHapticFeedback.current
    // 触边拉伸/回弹反馈（平台不支持时返回 null，自然退化为无）。
    val overscroll = rememberOverscrollEffect()

    // 双指缩放调整的字号（sp）。行高、gutter、layout 随之联动重算。整个缩放手势里字号**不变**，松手才由
    // commitZoom 换一次字号（见下方缩放 pointerInput）——手势期只动下面的 previewScale，零重排。
    var fontSizeSp by remember { mutableFloatStateOf(14f) }
    // 手势期连续缩放系数（相对手势起始字号），**仅在 draw 阶段读**（传给 EditorCanvas/CursorOverlay 的 provider）：
    // 每指针事件写它只使画布重绘、不触发重组/重排。恒 1f 表示非缩放态；松手 commitZoom 后归 1、换成真实字号。
    var previewScale by remember { mutableFloatStateOf(1f) }
    // 缩放预览的 draw 阶段平移（屏幕 px），与 previewScale 组成运行态仿射：screenX = s·px + previewTx、screenY = s·py + previewTy
    // （px/py 为相对当前 scroll 的内容坐标）。手势逐帧按「绕当前双指中点缩放 + 中点位移平移」增量累积（T' = ez·T + (c − ez·prevC)）
    // → 四向跟手；恒 (1,0,0) 表非缩放态。松手 commitZoom 把 (s, tx, ty) 折进真实 scroll（不换行自相似精确、换行走顶部锚）。换行下
    // 横向钉左（previewTx 恒 0）。**仅 draw 阶段读**，写它只重绘不重组/重排。
    var previewTx by remember { mutableFloatStateOf(0f) }
    var previewTy by remember { mutableFloatStateOf(0f) }
    // 换行提交的顶部锚字符：换行重排非自相似、不能靠 tx/ty 折算，改锚「预览末帧屏幕顶处」的文档字符，由 LaunchedEffect(fontSizeSp) 重排后放回顶。
    var zoomWrapAnchorPos by remember { mutableStateOf<TextPosition?>(null) }
    val lineHeightSp = fontSizeSp * 1.5f
    // trim = None 让 lineHeight 对「单行」也生效，否则默认 Trim.Both 会让单行退回字体自然高度，
    // 中文回退字体度量更大 -> 含中文的行更高、错位。Center 让内容在统一行高内居中。
    val lineHeightStyle = remember {
        LineHeightStyle(alignment = LineHeightStyle.Alignment.Center, trim = LineHeightStyle.Trim.None)
    }
    // Android 关闭 includeFontPadding，消除 CJK 回退字体导致的行内英文基线下偏。
    val platformTextStyle = remember { editorNoFontPaddingStyle() }
    val textStyle = remember(colors, fontSizeSp) {
        TextStyle(
            color = colors.foreground,
            fontFamily = FontFamily.Monospace,
            fontSize = fontSizeSp.sp,
            lineHeight = lineHeightSp.sp,
            lineHeightStyle = lineHeightStyle,
            // 线性字体度量 + 亚像素定位（去 hinting/网格拟合）：字形在不同字号间严格等比 → 双指缩放「缩放旧字形(预览)」与
            // 「原生新字形(提交)」逐像素一致，消除松手时的亚像素左右吸附。代价：静态小字号略软（高 DPI 上可接受）。
            textMotion = TextMotion.Animated,
            platformStyle = platformTextStyle
        )
    }
    val numberStyle = remember(colors, fontSizeSp) {
        TextStyle(
            color = colors.gutterForeground,
            fontFamily = FontFamily.Monospace,
            // 行号字号取正文的固定比例（而非 -1 常量偏移）：使 gutter 宽随字号线性缩放、布局自相似，缩放提交时行号不跳。
            fontSize = (fontSizeSp * NUMBER_FONT_SCALE).coerceAtLeast(6f).sp,
            lineHeight = lineHeightSp.sp,
            lineHeightStyle = lineHeightStyle,
            textMotion = TextMotion.Animated, // 同正文：线性度量 + 亚像素，行号缩放时也不抖
            platformStyle = platformTextStyle
        )
    }
    val lineHeightPx = with(density) { lineHeightSp.sp.toPx() }
    // padX 随字号成比例（默认 14sp→8dp），使内边距与 gutter 宽在缩放下自相似——预览等比缩放与松手重排几何一致、行号不跳。
    val padXPx = with(density) { (PAD_X_BASE_DP * fontSizeSp / LAYOUT_BASE_FONT_SP).dp.toPx() }
    // 拖动手柄半径与命中外扩（细光标线太小，靠 slop 放大可抓区域）。
    val handleRadiusPx = with(density) { 8.dp.toPx() }
    val handleSlopPx = with(density) { 10.dp.toPx() }
    // 统一基线：各行把字母基线钉到「含 CJK 参考行」的 firstBaseline（参考串须含 CJK）。等于每行都预留 CJK 竖直度量：
    // 英数位置恒定（加删中文不跳）、CJK 落在自然偏下位置不上飘。绘制顶 = 行顶 + (refBaselinePx − 本行 firstBaseline)。
    val refBaselinePx = remember(textStyle) { measurer.measure("Ag中", textStyle).firstBaseline }

    // M2：超长「网格行」判定 + 等宽字符宽。超阈值的行不整行 shaping，只按可见列窗口做等宽算术定位/绘制
    // （仅不换行；softWrap 下仍走整行折行测量）。gridRef = 参考字符 "0" 的 layout：垂直度量复用其 cursorRect(0)。
    // charW 取其**精确子像素 advance**（末端光标 x = cursorRect(1).left），不用 size.width——那是整数取整的盒宽。
    // 网格行用 col*charW 定位光标/命中/切片原点，而切片文本按真实 advance 排布；用取整宽会逐列累积漂移
    // （每字差零点几像素），长行右段光标与字形明显错位、点选/编辑落到隔壁列。取真实 advance 后三者与字形同尺。
    val gridRef = remember(textStyle) { measurer.measure("0", textStyle) }
    val charWpx = remember(gridRef) { gridRef.getCursorRect(1).left }
    val gridRefBaseline = gridRef.firstBaseline
    val gridRefCursor = remember(gridRef) { gridRef.getCursorRect(0) }
    // 网格快路仅适用「窄行」（全为可打印 ASCII、等宽半角）：其等宽算术 col*charW 与 ASCII 基线 gridRefBaseline
    // 只有在此前提下才成立。含 CJK/全角/emoji 的长行——字形非等宽（横向漂移）、度量非 ASCII（竖直错位）——据此
    // 退回整行 shaping（layoutFor），走与普通行相同的精确 layout（光标/命中/CJK 基线对齐都对）。是否窄行按行缓存、
    // version 未变即命中；编辑后按「长度+hash 指纹」重校（不把整行内容留在缓存里）。含 CJK 的超长行退回 shaping 会整行重测，属罕见代价。
    // 以 engine 为 key：version 计数是每 buffer 独立的小整数（新开文档都从 2 起），换 controller 时
    // 若沿用旧缓存，version 恰好相等会让快路径直接复用**上一份文档**的条目——必须随 engine 重建。
    val narrowCache = remember(engine) { LruCache<Int, NarrowFlag>(256) }
    fun isNarrowLine(line: Int): Boolean {
        val version = engine.buffer.version
        val cached = narrowCache[line]
        if (cached != null && cached.validatedVersion == version) return cached.narrow
        val content = engine.buffer.lineText(line)
        // 指纹（长度+hash）相符即视作未变、刷新版本戳复用结果：省下把整行内容长期钉在缓存里。同长且同 hash 的
        // 不同内容概率可忽略，即便撞上也只是本行一次错判、下次真实编辑（指纹必变）即纠正。
        if (cached != null && cached.length == content.length && cached.hash == content.hashCode()) {
            cached.validatedVersion = version
            return cached.narrow
        }
        val narrow = isNarrowGridText(content)
        narrowCache[line] = NarrowFlag(version, content.length, content.hashCode(), narrow)
        return narrow
    }

    // 长度先短路：短行不进 isNarrowLine、不触发整行读取，只有超阈值的长行才判窄。
    fun isGridLine(line: Int): Boolean =
        !softWrap && charWpx > 0f && engine.buffer.lineLength(line) > LONG_LINE_THRESHOLD && isNarrowLine(line)

    // 读取 buffer.version（快照 state）订阅编辑：内容/行数变化后整体重组，gutter 宽、内容高、可见窗口随之
    // 刷新。此读取本身即订阅，勿删。逐行 layout 缓存不再以它失效（否则每键整表重测），失效见 layoutFor。
    engine.buffer.version
    val lineCount = engine.buffer.lineCount

    // 语法高亮：显式插件优先，否则按 language 选内置。行状态链缓存随插件/引擎重建。
    val resolvedHighlighter = highlighter ?: when (language) {
        EditorLanguage.Yaml -> remember { YamlHighlighter() }
        EditorLanguage.PlainText -> null
    }
    val highlightCache = remember(resolvedHighlighter, engine) { resolvedHighlighter?.let { HighlightCache(it) } }
    // 编辑后的状态链失效必须在「取 spans 之前」完成，按 version 去重（非 state、UI 线程独占读写）。
    // 不能只在组合里做：keep-in-view 收集器 / 边缘自动滚动等组合外路径也会在编辑后立刻调 layoutFor，
    // 若彼时还没截断状态链，会用旧状态算出错误 spans 并以新 version 入 layout 缓存——回头快路径按
    // version 命中，错误着色一直钉到下次编辑。故组合内与 layoutFor 内都先经此兜底。
    val highlightSeenVersion = remember(highlightCache) { intArrayOf(-1) }
    fun ensureHighlightFresh() {
        if (highlightCache != null && highlightSeenVersion[0] != engine.buffer.version) {
            highlightSeenVersion[0] = engine.buffer.version
            engine.consumeDirty()?.let { d -> highlightCache.invalidate(d.from, d.endExclusive, d.structural) }
        }
    }
    ensureHighlightFresh()

    val gutterDigits = EditorGeometry.gutterDigits(lineCount)
    val gutterWidthPx = remember(gutterDigits, numberStyle) {
        measurer.measure("0".repeat(gutterDigits), numberStyle).size.width + padXPx * 2
    }

    // 每帧高频写读的量用 primitive FloatState，避免装箱 Float（滚动/拖拽每帧几个装箱分配纯属浪费）。
    var scrollY by remember { mutableFloatStateOf(0f) }
    var scrollX by remember { mutableFloatStateOf(0f) }
    var viewportWidth by remember { mutableFloatStateOf(0f) }
    var viewportHeight by remember { mutableFloatStateOf(0f) }
    // 编辑器内容区顶边在窗口中的 y（放大镜 Popup 用它把胶囊上浮到工具栏/状态栏空间、不越出窗口顶）。IME/旋转会变，实时更新。
    var contentTopInWindow by remember { mutableFloatStateOf(0f) }

    // 换行模式下正文可用宽度（测量宽度约束）。
    val textAreaWidthPx = (viewportWidth - gutterWidthPx - padXPx * 2).coerceAtLeast(1f)
    val widthBucket = if (softWrap) textAreaWidthPx.toInt() else 0

    // 视觉行索引：仅换行模式使用。不换行模式 lineTopPx/lineAtPx 走平凡公式、从不读它，故固定大小 1。
    // 换行模式按 (宽度/换代) 重建，但**不以字号为 key**——否则双指缩放每过一档就 O(n) 重建 IntArray(n)+buildTree，
    // 超大 softWrap 文件缩放会剧烈掉帧。字号变时保留本索引：可见行经 layoutFor 在新字号下重测并 setRows 增量更新
    // （O(log n)/行、有界），视口外行暂留旧字号行数作估算、滚动时自然收敛——即「换行下缩放不逐帧全量重折、只增量」。
    // **也不以行数为 key**：行数变化（回车/删行）经引擎的 LineSplice 队列增量 splice（编辑区间外的已测量
    // 行数保留、只有新行落 1 行估算），整表重建会把全文档打回估算、内容高度骤变引起视口跳动。
    // 创建时弃置积压 splice：建索引前的编辑已反映在当前行数里，重放会错位。
    val rowIndex = remember(softWrap, widthBucket, engine.contentGeneration) {
        VisualRowIndex(if (softWrap) engine.buffer.lineCount else 1).also { engine.consumeLineSplices() }
    }

    // 编辑后、任何按行号读几何之前，先把积压的行结构变化应用到行索引。组合本身与组合外路径都要经过
    // （layoutFor / lineTopPx / lineAtPx——keep-in-view 收集器、边缘自动滚动会在编辑后、重组前就读几何）。
    // 非换行模式索引恒 size-1、从不读，但仍消费队列防积压。空队列时只是一次 isEmpty 检查，热路径可担。
    fun drainRowSplices() {
        val splices = engine.consumeLineSplices()
        if (softWrap) for (s in splices) rowIndex.splice(s.startLine, s.oldLines, s.newLines)
    }
    drainRowSplices()

    // 逐行 layout 缓存：只在宽度/模式/字号/配色/插件变化时整表失效，不再以 version 失效——否则每敲一个字符
    // 整表丢弃、可见行全部重测。失效改由下方按行「内容 + 着色段」比对精确处理（本行被编辑、或上游块标量
    // 状态改变本行着色才重测；插入/删除行的下标平移也会因内容不符自然重测）。有界 LRU：超上限淘汰最久未用。
    // engine 入 key 同 narrowCache：跨文档的 version 快路径不可比。
    val layoutCache = remember(engine, softWrap, widthBucket, fontSizeSp, colors, resolvedHighlighter) {
        LruCache<Int, CachedLayout>(4096)
    }

    fun layoutFor(line: Int): TextLayoutResult? {
        if (line < 0 || line >= engine.buffer.lineCount) return null
        if (isGridLine(line)) return null // 网格行不整行测量：绘制走可见列切片（EditorCanvas），几何走等宽算术
        ensureHighlightFresh() // 组合外调用（keep-in-view 等）也先截断状态链，见声明处
        drainRowSplices() // 同上：行结构变化也须在按行号回填 rowIndex 之前生效
        val version = engine.buffer.version
        val cached = layoutCache[line]
        // 快路：version 未变说明自上次校验以来无编辑 → 行文本与着色必然未变，跳过物化与 O(len) 比较，零分配。
        if (cached != null && cached.validatedVersion == version) {
            // 命中也回填视觉行数：宽度/换代重建后、以及 splice 落下的新行都还是 1 行估算，而命中
            // 分支不经下面的 setRows，需在此补上，否则软换行的行高/定位会退回估算值。
            if (softWrap) rowIndex.setRows(line, cached.layout.lineCount)
            return cached.layout
        }
        // version 变了（发生过编辑）：按「内容 + 着色段」精确校验。lineText 至多取一次；着色段经状态链缓存
        // 取（分词便宜）。都相符 → 刷新版本戳复用 layout；否则重测。
        val content = engine.buffer.lineText(line)
        val spans = highlightCache?.spansForLine(line) { engine.buffer.lineText(it) } ?: emptyList()
        if (cached != null && cached.content == content && cached.spans == spans) {
            cached.validatedVersion = version
            if (softWrap) rowIndex.setRows(line, cached.layout.lineCount)
            return cached.layout
        }
        val annotated = highlightedText(content, spans, colors.syntax)
        val measured = if (softWrap) {
            measurer.measure(
                annotated,
                textStyle,
                softWrap = true,
                constraints = Constraints(maxWidth = textAreaWidthPx.toInt().coerceAtLeast(1))
            )
        } else {
            measurer.measure(annotated, textStyle, softWrap = false)
        }
        layoutCache[line] = CachedLayout(version, content, spans, measured)
        if (softWrap) rowIndex.setRows(line, measured.lineCount)
        return measured
    }

    // 行 -> 顶部像素 / 像素 -> 行（换行走视觉行索引，不换行走平凡公式，行为不变）。
    fun lineTopPx(line: Int): Float {
        if (softWrap) drainRowSplices() // 编辑后重组前就可能被读（keep-in-view 等），先把行结构变化落进索引
        return if (softWrap) rowIndex.rowsBefore(line) * lineHeightPx else line * lineHeightPx
    }

    fun lineAtPx(y: Float): Int {
        if (softWrap) drainRowSplices()
        val row = (y / lineHeightPx).toInt().coerceAtLeast(0)
        return if (softWrap) rowIndex.lineAtRow(row) else row.coerceIn(0, (lineCount - 1).coerceAtLeast(0))
    }

    // 翻页行数：约一屏可见行减 1 行重叠（至少 1）。viewportHeight/lineHeightPx 为实时几何。
    fun pageLines(): Int = ((viewportHeight / lineHeightPx).toInt() - 1).coerceAtLeast(1)

    // 量化滚动：组合只订阅「首个可见行」，跨行才重组；同一行内的滚动仅由 EditorCanvas 在 draw 阶段读
    // 像素级 scrollY/scrollX 平滑重绘，不再每滚 1px 全量重组。derivedStateOf 仅在整数结果变化时通知读者。
    // lineCount 必须作 key：derivedStateOf 的 lineAtPx 闭包捕获 lineCount 做 coerceIn 上界，行数变化（尤其
    // 初始 setText 把 1→N）时须重建闭包，否则捕获旧上界（如 1）会把 scrollLine 永远钳到 0、预测量窗口不前移，
    // 靠下的长/网格行不计入 widestSeen、横向滚动范围被卡死（不换行模式下 rowIndex 恒定、不再连带触发重建）。
    val scrollLine by remember(rowIndex, lineHeightPx, softWrap, lineCount) { derivedStateOf { lineAtPx(scrollY) } }

    // 预测量可见窗口（组合阶段填充缓存 + 行索引），并求不换行下最宽行以定横向滚动范围。
    val firstVisibleLine = (scrollLine - 3).coerceAtLeast(0)
    val approxRows = (viewportHeight / lineHeightPx).toInt() + 8
    val measureEnd = (firstVisibleLine + approxRows).coerceAtMost((lineCount - 1).coerceAtLeast(0))
    // 横向范围以「已测量过的最宽行」为准、只增不减（长行纵向滚出可见区后 maxScrollX 不骤缩、不把 scrollX 夹回，视口不瞬移；
    // 同 Monaco 维护 longest line）。非 state，写读均在组合内、不触发重组。engine.contentGeneration 作 key：换文档时重置。
    // **不以 fontSizeSp 为 key**：否则双指缩放换字号会 remember 重置、只从「当前可见行」重填——若最宽行（超长网格行）此刻已滚出
    // 视口，widestSeen 骤缩为可见短行、maxScrollX 崩塌 → 缩放松手时 scrollX 被夹到可见行右缘、横向大跳。改为跨字号保留：
    // widestSeen[1] 记上次测量字号，字号变时把宽度 [0] 按比例缩放（等宽 advance / layout 宽 ∝ 字号），随后可见行只增补。
    val widestSeen = remember(softWrap, widthBucket, engine.contentGeneration) { floatArrayOf(0f, fontSizeSp) }
    if (widestSeen[1] != fontSizeSp) {
        if (widestSeen[1] > 0f) widestSeen[0] *= fontSizeSp / widestSeen[1]
        widestSeen[1] = fontSizeSp
    }
    for (ln in firstVisibleLine..measureEnd) {
        if (isGridLine(ln)) {
            val w = engine.buffer.lineLength(ln) * charWpx // 网格行宽度算术得出，不整行测量
            if (w > widestSeen[0]) widestSeen[0] = w
        } else {
            val l = layoutFor(ln)
            if (!softWrap) {
                // 行宽取 getLineRight（shaping 后的真实墨迹右缘）而非 size.width：后者在无限宽约束下走
                // intrinsic 宽度路径取整，与逐簇布局右缘不一致——CJK/ASCII 混排百余字符可累计低估上百像素，
                // maxScrollX 随之偏短、长行尾部滚不出来（charW 在声明处早为同一理由避开了 size.width）。
                val w = l?.getLineRight(0) ?: 0f
                if (w > widestSeen[0]) widestSeen[0] = w
            }
        }
    }

    val contentHeight = (if (softWrap) rowIndex.totalRows() else lineCount) * lineHeightPx
    // 两个纵向上界（见 [BOTTOM_SCROLL_PAD_FRACTION]）：maxScrollY 含底部留白，供**主动滚动/渲染/命中/缩放预览**
    // （可滚过文末、末行上移下方留空）；maxScrollYSettle 是不含留白的自然底（末行贴底），仅供**视口变大时的 settle**
    // （收键盘/转屏变高）夹回贴底、不残留留白。留白用连续式 `(内容高−视口+留白).coerceAtLeast(0)`（非「溢出才加」的分段）：
    // 临界字号平滑、焦点滚动量落钳制内不被夹回。缩放预览的纵向上界同样含留白（与提交 re-clamp 同式 ⇒ 松手不跳、起手不 snap）。
    val bottomScrollPadPx = viewportHeight * BOTTOM_SCROLL_PAD_FRACTION
    val maxScrollY = (contentHeight - viewportHeight + bottomScrollPadPx).coerceAtLeast(0f)
    val maxScrollYSettle = (contentHeight - viewportHeight).coerceAtLeast(0f)
    val maxScrollX = if (softWrap) 0f else {
        (gutterWidthPx + padXPx * 2 + widestSeen[0] + viewportWidth * RIGHT_SCROLL_PAD_FRACTION - viewportWidth)
            .coerceAtLeast(0f)
    }

    // 换行下缩放的顶部锚定精确校正：字号现每手势只在 commitZoom 变一次 → 本效应每手势只触发一次。commitZoom 已同步置好近似
    // scrollY（提交帧≈预览末帧）；本效应在可见行按新字号重排后，用新布局把「锚字符所在折行」精确放回视口顶 (y=0)——顶部稳定、
    // 下方重排（比锚焦点字符可预测，焦点在长折行里的新位置难料、易 settle）。k 公式对换行非均匀折行不成立，故换行走「顶部字符
    // 锚定 + post-layout 校正」。re-clamp 效应声明在本效应之后（它要读滚动活动信号），但只做范围夹：
    // 本效应的锚定值自身已 coerce 在范围内、不会被改写。不换行 scrollY 已由 k 公式精确设好、不进本效应。
    LaunchedEffect(fontSizeSp) {
        if (!softWrap) return@LaunchedEffect
        val ap = zoomWrapAnchorPos ?: return@LaunchedEffect
        val layout = layoutFor(ap.line) ?: return@LaunchedEffect
        val col = ap.column.coerceIn(0, engine.buffer.lineLength(ap.line))
        val vr = layout.getLineForOffset(col) // 锚字符所在视觉行（新字号 layout）
        val anchorRowTopY = lineTopPx(ap.line) + layout.getLineTop(vr) // 该折行顶在内容坐标里的 y
        scrollY = anchorRowTopY.coerceIn(0f, maxScrollY) // 回到视口顶 (screenY=0)；上界含留白，缩放不夹掉主动滚入的留白
    }

    // ── 滚动条：显示状态机 ─────────────────────────────────────────────────────────────────
    // 显示只由「用户指针滚动」驱动（scroll2D 回调 / 滚轮 / 缩放后单指平移 / thumb 拖拽自身）；程序性
    // scrollY 变化（keep-in-view、goto/查找跳转、收键盘 re-clamp）**不显示**——否则打字、收键盘都会在
    // 右缘凭空出现一个短暂的点击拦截区。机制是沿触发而非观测 scrollY：fling 每帧写 scrollY，snapshotFlow
    // 观测它会每帧装箱发射 + collectLatest 每帧重启动画协程，恰好砸在本文件最严的热路径上；这里滚动
    // 回调只写普通时间戳（零分配），仅「隐藏→显示」的上升沿写一次 tick state。
    val scrollbarAlpha = remember(engine) { Animatable(0f) }
    val scrollbarShowTick = remember(engine) { mutableIntStateOf(0) }
    val scrollbarDragging = remember(engine) { mutableStateOf(false) } // draw 读（加宽过渡）+ 长按门 + 看门恒显
    val scrollbarThumbTopOverride = remember(engine) { mutableFloatStateOf(-1f) } // 拖拽期 thumb 跟手位置；-1 = 按比例
    val scrollbarShownState = remember(engine) { mutableStateOf(false) } // 组合读：可抓期挂载右缘系统手势排除
    val scrollbarClock = remember(engine) { ScrollbarClock() }

    fun markUserScroll() {
        scrollbarClock.last = TimeSource.Monotonic.markNow()
        if (!scrollbarClock.shown || scrollbarClock.fading) {
            scrollbarClock.shown = true
            scrollbarClock.fading = false
            scrollbarShowTick.intValue++ // 低频沿：已显示且未在淡出时只刷时间戳、零 state 写
        }
    }

    LaunchedEffect(engine) {
        snapshotFlow { scrollbarShowTick.intValue }.collectLatest { tick ->
            if (tick == 0) return@collectLatest // 挂载首发不显示（打开文件不闪条）
            scrollbarShownState.value = true
            scrollbarAlpha.animateTo(1f, tween(100))
            while (true) {
                withFrameNanos { }
                if (scrollbarDragging.value || scrollbarClock.hovering) {
                    scrollbarClock.last = TimeSource.Monotonic.markNow() // 拖拽/悬停恒显 = 计时持续顺延
                    continue
                }
                if (scrollbarClock.last.elapsedNow() >= SCROLLBAR_IDLE_MS.milliseconds) break
            }
            scrollbarClock.fading = true
            scrollbarAlpha.animateTo(0f, tween(300))
            // 走到这 = 淡出未被打断（淡出中 markUserScroll 会 tick++，collectLatest 在 animateTo 挂起点
            // 取消本块重启、alpha 从中间值续入淡入，下面两行不执行）。
            scrollbarClock.fading = false
            scrollbarClock.shown = false
            scrollbarShownState.value = false
        }
    }

    // 二维自由平移：跟手拖动时横纵可斜向同时滚（而非被锁在单轴——两个正交的单轴 scrollable 会在拖动起始按
    // 方向锁定其一）；但松手 fling 惯性时锁到主导轴（垂直或水平），避免斜向漂移。用独立 interactionSource 追踪
    // 「是否正在拖动」：DragInteraction 期间为跟手，其后由 fling 驱动的回调即惯性阶段。holder 用普通数组（非
    // State）避免每帧重组——滚动回调与 collect 都在 UI 线程、无并发。
    val scrollInteraction = remember { MutableInteractionSource() }
    val draggingHolder = remember { booleanArrayOf(false) }
    val flingAxis = remember { intArrayOf(0) } // 0=未定, 1=纵, 2=横；每次新拖动清零，fling 首帧按速度方向锁定
    LaunchedEffect(scrollInteraction) {
        scrollInteraction.interactions.collect { i ->
            draggingHolder[0] = when (i) {
                is DragInteraction.Start -> true
                is DragInteraction.Stop, is DragInteraction.Cancel -> false
                else -> draggingHolder[0]
            }
        }
    }

    // 视口变大 / 内容变短使 maxScroll 缩小时，把滚动量夹回范围内。否则在文档底部弹出输入法（视口被 imePadding
    // 压小、scrollY 被 keep-in-view 推大）后收起输入法，视口复原、maxScrollY 骤减，而 scrollY 残留旧大值：
    // firstVisibleLine 读未夹的 scrollY 会指到更靠下的行，绘制偏移却读已夹的 scrollY，二者错位 → 上方留空白。
    // 视口「变大」（收键盘 / 转屏变高）→ 夹到自然底 maxScrollYSettle（末行贴底、不残留留白，修 IME 收键盘留白）；
    // 其余（缩放/编辑改内容高、弹键盘缩小视口）→ 夹到含留白的 maxScrollY，**缩放/编辑不夹掉主动滚入的底部留白、不跳**。
    // 按「上一帧视口高」判定「变大」，确定性、不依赖 effect 执行顺序。
    // **滚动进行中贴底修正降级为宽松夹**：收键盘是多帧 insets 动画、本效应以 viewportHeight 为 key 逐帧
    // 重启，而拖动/fling 的回调也在每帧写 scrollY（宽松上界、底部区域正落在留白带里）——逐帧把它拽回
    // Settle 就是两个写者交替竞写，滚动肉眼抽搐。宽松夹已防住上面的错位空白（只需在范围内）；贴底只是
    // 静止时的美观修正。「滚动进行中」= 正在拖动，或滚动条的活动时间戳刚被刷新（拖动/fling 每帧都刷它）。
    val prevViewportH = remember { floatArrayOf(0f) }
    LaunchedEffect(maxScrollY, maxScrollX, viewportHeight) {
        val grew = viewportHeight > prevViewportH[0]
        prevViewportH[0] = viewportHeight
        val scrolling = draggingHolder[0] ||
                scrollbarClock.last.elapsedNow() < SCROLLBAR_QUIET_GRAB_MS.milliseconds
        scrollY = scrollY.coerceIn(0f, if (grew && !scrolling) maxScrollYSettle else maxScrollY)
        scrollX = scrollX.coerceIn(0f, maxScrollX)
    }
    val scroll2D = rememberScrollable2DState { delta ->
        markUserScroll() // 用户拖动/fling 每帧到此：唤出滚动条并顺延淡出（零分配，state 只在显隐沿写）
        if (draggingHolder[0]) {
            // 跟手：横纵自由平移。每帧清 flingAxis，松手后由 fling 首帧重新按速度方向锁轴。
            flingAxis[0] = 0
            val cx = (scrollX - delta.x).coerceIn(0f, maxScrollX)
            val cy = (scrollY - delta.y).coerceIn(0f, maxScrollY)
            val consumed = Offset(scrollX - cx, scrollY - cy)
            scrollX = cx; scrollY = cy
            consumed
        } else {
            // fling：首帧 delta 方向≈松手速度方向，据 |dx| vs |dy| 锁主轴、之后固定；次轴 delta 声称消费但不移动，
            // 既不斜向漂移、也不触发次轴 overscroll 发光。主轴撞边时未消费部分交 overscroll，正常显示到底反馈。
            if (flingAxis[0] == 0) flingAxis[0] = if (abs(delta.y) >= abs(delta.x)) 1 else 2
            if (flingAxis[0] == 1) {
                val cy = (scrollY - delta.y).coerceIn(0f, maxScrollY)
                val consumedY = scrollY - cy
                scrollY = cy
                Offset(delta.x, consumedY)
            } else {
                val cx = (scrollX - delta.x).coerceIn(0f, maxScrollX)
                val consumedX = scrollX - cx
                scrollX = cx
                Offset(consumedX, delta.y)
            }
        }
    }

    // 选区拖拽状态（提升到组合级，供边缘自动滚动 effect 读取）。
    // selectionAnchor：字符级锚（手柄端点拖拽）；selectionWordAnchor：词级锚（长按选词后按词扩展）。
    var selectionAnchor by remember { mutableStateOf<TextPosition?>(null) }
    var selectionWordAnchor by remember { mutableStateOf<TextRange?>(null) }
    // selectionLineAnchor：行级锚（鼠标三击选行后按行扩展）——仅鼠标块设置；边缘自动滚动 effect 据此按整行延伸。
    var selectionLineAnchor by remember { mutableStateOf<Int?>(null) }
    // 逐帧写入的拖拽坐标：只被 effect 协程体轮询，组合/绘制均不读——故写它不触发任何重组。
    var selectionDragPos by remember { mutableStateOf<Offset?>(null) }
    // 只在拖拽「起/止」翻转的布尔，专作边缘自动滚动 effect 的 key。不能用 `selectionDragPos != null` 作 key：
    // 那会在组合期读逐帧写入的 selectionDragPos，使拖选的每次移动都整体重组（P1）。
    var selectionDragActive by remember { mutableStateOf(false) }
    // 光标手柄拖拽的边缘自动滚动状态（与选区同机制，但落的是光标而非选区）：手柄拖到视口边缘热区时持续滚动、
    // 让光标能拖到当前不可见的内容处（否则光标被 positionAt 钳在可见带内、拖不过边）。caretDragPos 逐帧写、只被
    // effect 轮询（组合/绘制不读，写它不重组），已含手柄纵向抓取偏移 grabDy。caretDragActive 仅起/止翻转、作 effect key。
    var caretDragPos by remember { mutableStateOf<Offset?>(null) }
    var caretDragActive by remember { mutableStateOf(false) }
    // 放大镜显示条件：仅「光标手柄 / 选区端点手柄」拖拽时（下方最内层 pointerInput 的两支）为真——不含长按选择拖拽
    // （那支只置 selectionDragActive）。逐帧被放大镜层在 draw 读、只起/止翻转 → 不重组。
    var handleDragActive by remember { mutableStateOf(false) }

    // 最近一次指针交互是否来自鼠标：鼠标块置 true、触屏（点按 / 长按）置 false。为 true 时隐藏泪滴光标手柄与选区端点手柄
    // （桌面/鼠标场景不需要触摸取词手柄；放大镜本就只在触屏手柄拖拽的 handleDragActive 下出现，故不受影响）。
    var lastInteractionWasMouse by remember { mutableStateOf(false) }

    // 触屏文本操作悬浮条（选区上方浮条 / 点光标粘贴气泡）的「已触发」闩：点按落光标 / 选词 / 拖拽结算后置真，
    // 新一次触屏按下 / 编辑 / 切到鼠标 / 选中某项后置假。是否真正显示还要过 SelectionActionToolbar 的 show 门
    // （再排除拖手柄 / 框选 / 鼠标态）。
    var showTouchMenu by remember { mutableStateOf(false) }
    // 桌面右键上下文菜单：显示开关 + 锚点（编辑器局部像素，取右键按下点）。
    var showContextMenu by remember { mutableStateOf(false) }
    var contextMenuAnchor by remember { mutableStateOf<Offset?>(null) }

    var blink by remember { mutableStateOf(true) }
    // 选区变化即重置闪烁相位（光标一移立即可见）。用 snapshotFlow 在快照观察者里读 selection，而非把它当组合期
    // effect key——否则拖选每次移动都会整体重组本可组合（含预测量循环）。collectLatest 在新选区到达时取消上一
    // 轮闪烁循环，等价于「每次 selection 变化重启」。draw 里的 caretVisible 仍逐帧读 blink（重绘而非重组）。
    LaunchedEffect(readOnly) {
        if (readOnly) return@LaunchedEffect
        snapshotFlow { engine.selection }.collectLatest {
            blink = true
            while (true) {
                delay(500.milliseconds); blink = !blink
            }
        }
    }

    // 光标泪滴手柄：点按落光标 / 拖动后短暂显示，约 4s 无操作自动隐藏；打字（buffer.version 变化）立即收起。
    // 选区两端手柄不走这里——它们随选区常驻，由画布按 !selection.isEmpty 直接判定。
    var caretHandleVisible by remember { mutableStateOf(false) }
    var caretHandleToken by remember { mutableIntStateOf(0) }
    fun pingCaretHandle() {
        caretHandleVisible = true; caretHandleToken++
    }
    // 4s 无操作自动隐藏。用 snapshotFlow 观察 token，不把它当组合期 effect key——否则每次点按落光标/抓放手柄
    // （pingCaretHandle 自增 token）都多一次整体重组。collectLatest 在新 ping 到达时取消上一轮 delay = 重置计时。
    LaunchedEffect(Unit) {
        snapshotFlow { caretHandleToken }.collectLatest { token ->
            if (token == 0) return@collectLatest
            delay(4000.milliseconds); caretHandleVisible = false
        }
    }
    LaunchedEffect(engine.buffer.version) {
        // 任何编辑都收起光标手柄与操作菜单；首帧亦触发，但此时本就不可见、无副作用。
        caretHandleVisible = false
        showTouchMenu = false
        showContextMenu = false
    }

    // 触屏悬浮条在「拖手柄 / 长按框选」结算后复现：观察 (handleDragActive || selectionDragActive) 的真→假沿，
    // 拖拽一结束就重新亮出工具条（拖拽期由 SelectionActionToolbar 的 show 门隐藏）。仅触屏、且确有过一次拖拽才触发
    // （wasActive 初值假 → 挂载首帧的假值不误亮）。
    LaunchedEffect(Unit) {
        var wasActive = false
        snapshotFlow { handleDragActive || selectionDragActive }.collectLatest { active ->
            if (wasActive && !active && !lastInteractionWasMouse) showTouchMenu = true
            wasActive = active
        }
    }

    // 光标/端点在其行内的内容 x（网格行等宽算术、其余走该行 layout）；无 layout 返回 null。keep-in-view 与两个边缘自动滚动
    // effect 共用，做「横向 clip-gate 露出」。
    fun caretContentXOf(line: Int, column: Int): Float? {
        val col = column.coerceAtMost(engine.buffer.lineLength(line))
        return if (isGridLine(line)) col * charWpx else layoutFor(line)?.getCursorRect(col)?.left
    }

    // clip-gate：把 [curScrollX] 夹到「让 [caretCX] 落在可见带 [x, x+textAreaW] 内、留 margin 前瞻」；已在带内则原样返回
    //（可见即不动，不做「近边即滚」——避免点按落在可见靠边处被推滚）。仅用于不换行（换行 maxScrollX=0）。
    fun revealScrollXFor(caretCX: Float, curScrollX: Float): Float {
        val textAreaW = (viewportWidth - gutterWidthPx - padXPx * 2).coerceAtLeast(1f)
        val margin = padXPx * 3
        return when {
            caretCX < curScrollX -> (caretCX - margin).coerceIn(0f, maxScrollX)
            caretCX > curScrollX + textAreaW -> (caretCX - textAreaW + margin).coerceIn(0f, maxScrollX)
            else -> curScrollX
        }
    }

    // keep-in-view 的露出逻辑。几何量（lineHeightPx/lineTopPx/layoutFor/caretContentXOf/revealScrollXFor/softWrap）随字号每帧
    // 重算，但下方 LaunchedEffect(Unit) 的协程只在挂载时启动一次，会**永久捕获挂载帧（14sp）的几何**（stale-capture）：缩放后
    // 它按旧 14sp 几何算 caretTop、却比实时（新字号尺度）scrollY，两坐标系差「挂载/当前 字号比」→ 纯点击把已可见光标误判越界、
    // 滚一大段（跳位 ∝ 字号比；默认 14sp 时比=1、无跳）。故用 rememberUpdatedState 把整段提到「当前帧」，collect 里调 .value()
    // 用最新字号几何——与 positionAtLive/caretRectLive/liveMaxScroll 同法（本项目到处规避此坑，唯独这里漏了）。
    val revealCaretIntoViewLive = rememberUpdatedState {
        // 跟随「活动端」(head) 而非归一化的 selEnd：Shift+上/左 反向扩选时 head 在选区顶端，视口须随之上滚。
        val caret = engine.caret
        val line = caret.line
        // softWrap 下露出光标所在的那一「视觉行」，而非整条可能高过视口的折行——否则长行会把光标顶出视口。
        val curRow = if (softWrap) {
            layoutFor(line)?.getLineForOffset(caret.column.coerceIn(0, engine.buffer.lineLength(line))) ?: 0
        } else 0
        val caretTop = lineTopPx(line) + curRow * lineHeightPx
        val caretBottom = caretTop + lineHeightPx
        if (caretTop < scrollY) scrollY = caretTop
        else if (caretBottom > scrollY + viewportHeight) scrollY = caretBottom - viewportHeight
        // 横向随动：不换行时若光标越出可见带则滚动露出（clip-gate：可见即不动、留 margin 前瞻）。纯纵向导航（目标列生效）
        // 时跳过——否则目标列在短/长行间夹变会让视口横向来回 snap，很突兀。
        if (!softWrap && viewportWidth > 0f && !engine.hasGoalColumn) {
            caretContentXOf(line, caret.column)?.let { scrollX = revealScrollXFor(it, scrollX) }
        }
    }

    // keep-in-view：光标随选区/视口变化滚动露出。用 snapshotFlow 观察 (selection, viewport)、不把 selection 当组合期 key（P1）。
    // 观测集并入「是否正在手柄拖拽」：拖拽期标志为真、下方直接 return（滚动由边缘自动滚动接管）；拖拽结束标志 true→false 令
    // snapshotFlow 再 emit 一次、门禁解除后复位一次（修「拖到短行后滚出屏、松手不复位」）。露出走上面的 rememberUpdatedState
    // （当前帧几何）；effect 以 engine 为 key——闭包捕获 engine，宿主换绑 controller 时必须重启、否则永远观测旧引擎的
    // tick/selection（reveal 通道死掉）；缩放/字号变化不换 engine、不重启，仍不会缩放后强行把视口滚到光标。
    LaunchedEffect(engine) {
        // revealTick 必须进发射值：snapshotFlow 按结构相等去重，编程定位跳到「原地」（重复点同一 lint
        // 错误、goto 跳当前行）时 selection 不变、只有 tick 在动——不进值就不发射、视口滚不回来。
        var first = true
        var lastSel = engine.selection
        var lastTick = engine.revealTick
        var lastVh = viewportHeight
        var lastVw = viewportWidth
        snapshotFlow {
            listOf(engine.revealTick, engine.selection, viewportHeight, viewportWidth, selectionDragActive || caretDragActive)
        }.collect {
            if (viewportHeight <= 0f) return@collect // 尺寸未定不消费 first：等真实尺寸那次再算首发
            if (selectionDragActive || caretDragActive) return@collect // 选区/光标手柄拖拽时由边缘自动滚动接管
            // 分辨发射原因。显式动作（selection/tick 有变，或首发——恢复的深处光标靠首发露出）无条件
            // 露出，编辑/方向键/跳转照旧打断滚动。纯视口尺寸变化只在「缩小」时露出：弹键盘可能盖住光标、
            // 须顶回可见区；「变大」（收键盘）不遮挡任何已见内容，一律不露——否则滚动中收键盘、fling 触底
            // 停下后，键盘 insets 动画的尾帧发射（滚动静默窗已过、拦不住）会把刚滚到的视口整个拽回光标处
            //（跳回原位）。缩小动画与滚动并发时（弹键盘瞬间仍在 fling）仍由静默窗拦住竞写。
            val sel = engine.selection
            val tick = engine.revealTick
            val explicit = first || sel != lastSel || tick != lastTick
            val shrank = viewportHeight < lastVh || viewportWidth < lastVw
            first = false
            lastSel = sel
            lastTick = tick
            lastVh = viewportHeight
            lastVw = viewportWidth
            if (!explicit) {
                if (!shrank) return@collect
                val scrolling = draggingHolder[0] ||
                        scrollbarClock.last.elapsedNow() < SCROLLBAR_QUIET_GRAB_MS.milliseconds
                if (scrolling) return@collect
            }
            revealCaretIntoViewLive.value()
        }
    }

    // 手势闭包由 pointerInput(engine) 固定、不随滚动重启，会按值捕获几何量。gutter 宽用 rememberUpdatedState
    // 提到当前帧；滚动量则在 positionAt 里直接读活的 scrollY/scrollX——既让命中测试始终对齐当前滚动，
    // 又不因组合阶段读取滚动量而每滚 1px 重组。
    val hitGutterWidth = rememberUpdatedState(gutterWidthPx)

    fun positionAtWithScroll(offset: Offset, sY: Float, sX: Float): TextPosition {
        val ln = lineAtPx(offset.y + sY)
        val localX = (offset.x - hitGutterWidth.value - padXPx + sX).coerceAtLeast(0f)
        if (isGridLine(ln)) {
            return TextPosition(ln, EditorGeometry.gridXToColumn(localX, charWpx, engine.buffer.lineLength(ln)))
        }
        val layout = layoutFor(ln)
        // 撤销绘制时的基线平移，换回该行 layout 自身坐标再命中。
        val shift = refBaselinePx - (layout?.firstBaseline ?: refBaselinePx)
        val layoutY = (offset.y + sY) - lineTopPx(ln) - shift
        val col = layout?.getOffsetForPosition(Offset(localX, layoutY)) ?: 0
        return TextPosition(ln, col.coerceAtMost(engine.buffer.lineLength(ln)))
    }

    fun positionAt(offset: Offset): TextPosition =
        positionAtWithScroll(offset, scrollY.coerceIn(0f, maxScrollY), scrollX.coerceIn(0f, maxScrollX))

    // 手势闭包由 pointerInput(engine) 固定，不随字号/换行/行高变化重启。用 rememberUpdatedState 让它
    // 始终调「当前帧」的 positionAt（内含最新的 lineHeightPx / 视觉行索引 / softWrap / layout）。
    val positionAtLive = rememberUpdatedState<(Offset) -> TextPosition> { positionAt(it) }

    // 只读态提升为「当前帧」值，供固定 key 的手势闭包读取，避免 readOnly 切换后闭包按旧值捕获。
    val readOnlyLive = rememberUpdatedState(readOnly)

    // 剪切/复制/粘贴/全选统一执行器：硬键盘 onKeyEvent、触屏悬浮条、桌面右键菜单三条路共用（readOnly 读活值，
    // 避免被固定 key 的手势闭包按旧值捕获）。
    val clipboardActions = remember(engine, clipboard, clipboardScope) {
        EditorClipboardActions(engine, clipboard, clipboardScope) { readOnlyLive.value }
    }

    // 光标/端点在屏幕上的矩形 [left, top, bottom]，供手柄命中测试。读活的 scroll/layout；用 rememberUpdatedState
    // 让固定 key 的手势闭包取到当前帧几何（与 positionAtLive 同理）。与画布 drawHandle 的坐标算法一致。
    fun caretScreenColumnRect(pos: TextPosition): FloatArray? {
        val col = pos.column.coerceIn(0, engine.buffer.lineLength(pos.line))
        val sY = scrollY.coerceIn(0f, maxScrollY)
        val sX = scrollX.coerceIn(0f, maxScrollX)
        if (isGridLine(pos.line)) {
            val base = lineTopPx(pos.line) - sY + (refBaselinePx - gridRefBaseline)
            val x = gutterWidthPx + padXPx - sX + col * charWpx
            return floatArrayOf(x, base + gridRefCursor.top, base + gridRefCursor.bottom)
        }
        val layout = layoutFor(pos.line) ?: return null
        val cr = layout.getCursorRect(col)
        val base = lineTopPx(pos.line) - sY + (refBaselinePx - layout.firstBaseline)
        return floatArrayOf(gutterWidthPx + padXPx - sX + cr.left, base + cr.top, base + cr.bottom)
    }

    val caretRectLive = rememberUpdatedState<(TextPosition) -> FloatArray?> { caretScreenColumnRect(it) }

    // 滚动条拖拽用的活值包装（pointerInput(engine) 闭包固定，字号/换行/行数变化不重启——同 caretRectLive 惯例）。
    val lineTopPxLive = rememberUpdatedState<(Int) -> Float> { lineTopPx(it) }
    val lineAtPxLive = rememberUpdatedState<(Float) -> Int> { lineAtPx(it) }
    val liveLineCount = rememberUpdatedState(lineCount)

    // 可见泪滴手柄命中（滚动条让位判定）：选区端点手柄的命中盒可整段泡在右缘热区里，可见手柄恒优先于
    // thumb——否则「长按选词拖到右缘、松手抓端点微调」会被 thumb 的绝对映射把文档传送走。分支与手柄
    // 拖拽块一致：有选区看两端，无选区看光标手柄（可见、非只读、非鼠标态）。
    fun hitsVisibleTouchHandle(p: Offset): Boolean {
        if (lastInteractionWasMouse) return false
        fun hit(kind: HandleKind, at: TextPosition): Boolean {
            val r = caretRectLive.value(at) ?: return false
            return EditorGeometry.handleGeometry(kind, r[0], r[1], r[2], handleRadiusPx, handleSlopPx).hitContains(p.x, p.y)
        }

        val sel = engine.selection
        return if (!sel.isEmpty) {
            hit(HandleKind.SelectionEnd, sel.end) || hit(HandleKind.SelectionStart, sel.start)
        } else {
            caretHandleVisible && !readOnlyLive.value && hit(HandleKind.Caret, sel.start)
        }
    }

    // 边缘自动滚动 effect 的 key 是 Boolean、不随重组刷新，循环体会按值捕获滚动上限。软换行下自动滚动
    // 驶入未测量区域时真实 maxScrollY 会随测量增大，用 rememberUpdatedState 让循环读到当前帧上限，
    // 否则会停在旧估算的「假底部」、选不到文档末尾（与 hitScrollY 同类修复）。
    val liveMaxScrollY = rememberUpdatedState(maxScrollY)
    val liveMaxScrollX = rememberUpdatedState(maxScrollX)
    // 滚轮步长用的实时行高（避开 pointerInput(Unit) 的 stale-capture；随字号缩放）。
    val liveLineHeightPx = rememberUpdatedState(lineHeightPx)
    // 缩放手势期做边界阻尼需按「预览缩放 s」算 maxScroll = s·内容尺寸 − 视口；内容尺寸经 rememberUpdatedState 提到当前帧，
    // 避开缩放手势 pointerInput(Unit) 闭包的 stale-capture（手势期无重排、内容尺寸恒定，故手势内取值稳定）。
    val liveContentHeight = rememberUpdatedState(contentHeight)
    val liveContentWidth = rememberUpdatedState(gutterWidthPx + padXPx * 2 + widestSeen[0])

    // 松手提交缩放：把连续 previewScale 落成精确的真实字号，一次性设好 scroll 并触发唯一一次重排。k = newFont/fontStart（== s，s 已夹字号范围）。
    // 不换行：自相似 + TextMotion ⇒ 新字号内容 == 旧内容×s 逐像素，把预览末帧的等效滚动 effScroll=s·baseScroll−t 折成新 scroll；手势期已把 effScroll
    // 硬钳在界内 ⇒ 必落界内、提交无跳。换行：横向钉左、纵向重排非自相似 → 锚「预览末帧屏幕顶」字符、由 LaunchedEffect(fontSizeSp) 重排后放回顶。
    fun commitZoom(fontStart: Float, s: Float, tx: Float, ty: Float) {
        val newFont = ZoomMath.commitFontSize(fontStart, s, ZOOM_MIN_SP, ZOOM_MAX_SP)
        // 字号与平移都极小（双指微动 / 纯噪声）：视作未动、不重排。
        if (abs(newFont - fontStart) < ZOOM_MIN_CHANGE_SP && abs(tx) < 0.5f && abs(ty) < 0.5f) return
        val k = newFont / fontStart // == s
        if (softWrap) {
            // 预览屏幕顶 y=0 对应旧内容 y：s·py+ty=0 → py=−ty/s → 旧内容 y = scrollY − ty/s（scrollY 手势期未变 = baseScrollY）。
            val topContentYOld = (scrollY - ty / s).coerceAtLeast(0f)
            zoomWrapAnchorPos = positionAtWithScroll(Offset(0f, 0f), topContentYOld, 0f)
            scrollY = ZoomMath.provisionalScrollYWrap(topContentYOld, k, 0f) // 提交帧≈预览末帧，效应再精确置顶
            scrollX = 0f
        } else {
            // 预览 screenY = s·(cy−scrollY)+ty，提交 screenY = s·cy − newScrollY ⇒ newScrollY = s·scrollY − ty（横向同理）。手势期已硬钳在界内，
            // coerceAtLeast(0) 仅防浮点/边界，上界由 re-clamp 在新字号下收。scrollX/scrollY 手势期未变 = baseScroll。
            scrollX = (s * scrollX - tx).coerceAtLeast(0f)
            scrollY = (s * scrollY - ty).coerceAtLeast(0f)
        }
        fontSizeSp = newFont // 唯一一次重排在此
        previewScale = 1f; previewTx = 0f; previewTy = 0f // 变换已折进 scroll，预览归位
    }
    // 手势闭包 key=Unit、不随重组重启，会按值捕获闭包。用 rememberUpdatedState 让它每次调「当前帧」的 commitZoom
    // （内含最新 softWrap / lineTopPx / layoutFor / liveMaxScroll），避开 stale-capture（与 positionAtLive 同理）。
    val commitZoomLive =
        rememberUpdatedState<(Float, Float, Float, Float) -> Unit> { fontStart, s, tx, ty -> commitZoom(fontStart, s, tx, ty) }

    // 选区拖拽到视口上/下/左/右热区时，按帧持续纵横滚动并同步延伸选区；速度随进入热区的深度线性增大。
    // 词锚（长按选词）按词粒度扩展、字符锚（手柄端点）按字符扩展。
    LaunchedEffect(selectionDragActive) {
        if (!selectionDragActive) return@LaunchedEffect
        while (true) {
            val pos = selectionDragPos ?: break
            val wordAnchor = selectionWordAnchor
            val charAnchor = selectionAnchor
            val lineAnchor = selectionLineAnchor
            if (wordAnchor == null && charAnchor == null && lineAnchor == null) break
            val maxY = liveMaxScrollY.value
            val maxX = liveMaxScrollX.value
            val stepY = edgeAutoScrollSpeed(pos.y, viewportHeight, lineHeightPx)
            var stepX = edgeAutoScrollSpeed(pos.x, viewportWidth, lineHeightPx)
            // 横向自动滚只在延伸端还能沿**当前行**推进时才进行：到行尾（右）/行首（左）即停，不把视口滚进该行文本外的空白
            // （当前行短而上下有更长行时，全局 maxScrollX>0 会误让其滚进空白、延伸端却被钳在行尾不动）。与光标手柄版同一道闸。
            // 并 gate maxX>0：maxX==0（softWrap 或无横向余量）时归零 stepX 也改不动 newX，跳过、免每帧 positionAtWithScroll/layoutFor。
            if (stepX != 0f && maxX > 0f) {
                val cur = positionAtWithScroll(pos, scrollY, scrollX)
                val lineLen = engine.buffer.lineLength(cur.line)
                if ((stepX > 0f && cur.column >= lineLen) || (stepX < 0f && cur.column <= 0)) stepX = 0f
            }
            val newY = if (stepY != 0f && maxY > 0f) (scrollY + stepY).coerceIn(0f, maxY) else scrollY
            var newX = if (stepX != 0f && maxX > 0f) (scrollX + stepX).coerceIn(0f, maxX) else scrollX
            // 拖拽期即时露出：延伸端被短行钳到当前不可见处时，立刻横滚把它拉回可见带（不等松手）。在 edge-step 之后再夹，
            // 故与之不打架——长行推边时端点就在指下、在带内、revealScrollXFor 原样返回；仅短行钳出屏外才真正横滚。
            if (!softWrap && maxX > 0f) {
                val c = positionAtWithScroll(pos, newY, newX)
                caretContentXOf(c.line, c.column)?.let { newX = revealScrollXFor(it, newX) }
            }
            if (newY != scrollY || newX != scrollX) {
                scrollY = newY
                scrollX = newX
                val caret = positionAtWithScroll(pos, newY, newX)
                when {
                    wordAnchor != null -> engine.selectWordRange(wordAnchor, caret)
                    lineAnchor != null -> {
                        val a = minOf(lineAnchor, caret.line)
                        val b = maxOf(lineAnchor, caret.line)
                        engine.setSelection(TextPosition(a, 0), TextPosition(b, engine.buffer.lineLength(b)))
                    }

                    else -> charAnchor?.let { engine.setSelection(it, caret) }
                }
            }
            withFrameNanos { }
        }
    }

    // 光标手柄拖拽的边缘自动滚动（与上方选区版同机制，但每帧落的是光标 setCursor 而非延伸选区）：手柄拖进边缘热区、
    // 或按住不动停在热区时，逐帧滚动并把光标落到滚动后指下的内容处——于是能把光标一路拖到当前视口外的内容。
    // 非热区时 step=0、不滚，光标由手势回调 setCursor 落定；二者同置光标、值一致、无冲突（与选区版对称）。
    LaunchedEffect(caretDragActive) {
        if (!caretDragActive) return@LaunchedEffect
        while (true) {
            val pos = caretDragPos ?: break
            val maxY = liveMaxScrollY.value
            val maxX = liveMaxScrollX.value
            val stepY = edgeAutoScrollSpeed(pos.y, viewportHeight, lineHeightPx)
            var stepX = edgeAutoScrollSpeed(pos.x, viewportWidth, lineHeightPx)
            // 横向自动滚只在光标还能沿**当前行**推进时才进行：到行尾（右）/行首（左）即停，不把视口滚进该行文本右侧的空白。
            // maxScrollX 是按最宽行算的全局上界；若当前行短（如 50 字符）而上下有更长行，全局 maxScrollX>0 会让 effect
            // 误以为「右边还能滚」，于是滚进空白、光标却被 positionAt 钳在行尾不动。按当前行长度就地闸停。
            // 并 gate maxX>0：maxX==0（softWrap 或无横向余量）时归零 stepX 也改不动 newX，跳过、免每帧 positionAtWithScroll/layoutFor。
            if (stepX != 0f && maxX > 0f) {
                val cur = positionAtWithScroll(pos, scrollY, scrollX)
                val lineLen = engine.buffer.lineLength(cur.line)
                if ((stepX > 0f && cur.column >= lineLen) || (stepX < 0f && cur.column <= 0)) stepX = 0f
            }
            val newY = if (stepY != 0f && maxY > 0f) (scrollY + stepY).coerceIn(0f, maxY) else scrollY
            var newX = if (stepX != 0f && maxX > 0f) (scrollX + stepX).coerceIn(0f, maxX) else scrollX
            // 拖拽期即时露出：光标被短行钳到当前不可见处时，立刻横滚把它拉回可见带（不等松手）。在 edge-step 之后再夹，与之
            // 不打架——长行推边时光标就在指下、在带内、revealScrollXFor 原样返回；仅短行钳出屏外才真正横滚。
            if (!softWrap && maxX > 0f) {
                val c = positionAtWithScroll(pos, newY, newX)
                caretContentXOf(c.line, c.column)?.let { newX = revealScrollXFor(it, newX) }
            }
            if (newY != scrollY || newX != scrollX) {
                scrollY = newY
                scrollX = newX
                engine.setCursor(positionAtWithScroll(pos, newY, newX))
            }
            withFrameNanos { }
        }
    }

    // softWrap 下按「视觉行」上下移动：一条文档行可能折成多视觉行，直接 line±1 会跨过整条长行、和用户看到的
    // 「下一行」不符。用当前帧 layout 求光标所在视觉行与像素 x，落到目标视觉行同 x 处的列；跨视觉行时进上一/
    // 下一文档行的末/首视觉行。desiredX 在连续上下移动间记忆（穿过更短的视觉行不丢）：仅当上次落点仍等于
    // 当前 head 时复用，否则按当前光标重新取 x（横向移动/编辑/点按都会让 head 偏离，从而自然重置）。
    var vGoalX by remember { mutableStateOf<Float?>(null) }
    var vGoalCaret by remember { mutableStateOf<TextPosition?>(null) }
    fun moveCaretVisual(dir: Int, extend: Boolean) {
        val from = engine.caret
        val layout = layoutFor(from.line) ?: run {
            engine.moveCaretVertically(dir, extend); return // 无 layout 时退回按文档行移动
        }
        val col = from.column.coerceIn(0, engine.buffer.lineLength(from.line))
        val curRow = layout.getLineForOffset(col)
        val goalX = if (vGoalX != null && vGoalCaret == from) vGoalX!! else layout.getCursorRect(col).left
        val step = EditorGeometry.visualVerticalTarget(from.line, curRow, dir, lineCount) { l -> layoutFor(l)?.lineCount ?: 1 }
            ?: return // 文档端，不动
        val targetLayout = if (step.line == from.line) layout else (layoutFor(step.line) ?: return)
        val yMid = (targetLayout.getLineTop(step.row) + targetLayout.getLineBottom(step.row)) / 2f
        val newCol = targetLayout.getOffsetForPosition(Offset(goalX, yMid))
            .coerceIn(0, engine.buffer.lineLength(step.line))
        val to = TextPosition(step.line, newCol)
        if (extend) engine.extendSelectionTo(to) else engine.setCursor(to)
        vGoalX = goalX
        vGoalCaret = to
    }

    // 底部安全区 = 导航栏 / captionBar / 键盘 三者较大者（union 取各边最大）。编辑器自管它：背景铺到屏幕边缘、
    // 内容抬到栏上，故宿主无需再加 imePadding/导航栏 padding。键盘收起=导航栏高，弹出=键盘高（已含导航栏区）。
    val bottomBarInsets = WindowInsets.navigationBars
        .union(WindowInsets.captionBar)
        .union(WindowInsets.ime)
        .only(WindowInsetsSides.Bottom)
    val showSymbolBar = !readOnly && symbols.isNotEmpty()
    val showBottomSymbolBar = showSymbolBar && symbolBarPosition == SymbolBarPosition.Bottom
    val showEndSymbolBar = showSymbolBar && symbolBarPosition == SymbolBarPosition.End
    val hasBottomBar = bottomBar != null
    val handlesBottomInsets = showBottomSymbolBar || hasBottomBar

    // 根为 Column：查找条停靠最上，文本区与右侧符号栏共用中间区域，底部工具栏统一处理系统栏安全区。
    Column(modifier.background(colors.background)) {
        // 停靠式查找/替换条：占自己的布局行，文本区随开合让位；不用 Popup——Android 上 focusable Popup
        // 会吞掉浮层外全部触摸（编辑区/宿主工具栏点不动），而输入框又必须可获焦收 IME。
        FindReplaceBar(
            session = findSession,
            colors = colors,
            readOnly = readOnly,
            onRequestEditorFocus = { focusRequester.requestFocus() },
        )
        GotoLineBar(
            session = gotoSession,
            lineCount = lineCount,
            colors = colors,
            onRequestEditorFocus = { focusRequester.requestFocus() },
        )
        Row(
            Modifier
                .fillMaxWidth()
                .weight(1f),
        ) {
            Box(
                Modifier
                    .weight(1f)
                    // 没有底部工具栏时文本区自行避让系统栏；有工具栏时由工具栏统一处理，避免重复留白。
                    .then(if (handlesBottomInsets) Modifier else Modifier.windowInsetsPadding(bottomBarInsets))
                    .clipToBounds()
                    .overscroll(overscroll)
                    .onSizeChanged { viewportWidth = it.width.toFloat(); viewportHeight = it.height.toFloat() }
                    .onGloballyPositioned { contentTopInWindow = it.positionInWindow().y }
                // 滚动条可抓期把右缘热区从系统手势区摘出来（Android 手势导航的返回区与热区重合，不摘则
                // 抓 thumb 起手稍带向左分量就被系统当返回、直接退出编辑屏）；淡出即撤销，不常驻削弱返回手势。
                .then(
                    if (scrollbarShownState.value) {
                        Modifier.editorRightEdgeGestureExclusion(with(density) { SCROLLBAR_HOT_ZONE.toPx() })
                    } else Modifier
                )
                .scrollable2D(scroll2D, overscrollEffect = overscroll, interactionSource = scrollInteraction)
                // 鼠标滚轮 / 触控板滚动：scrollable2D 只含拖拽、无滚轮节点（滚轮逻辑只在 1D scrollable 的
                // MouseWheelScrollingLogic 里），故自行处理 Scroll 事件、据 scrollDelta 更新滚动量。clamp 用实时
                // 上界 + 实时行高，避开 pointerInput(Unit) 的 stale-capture。
                .pointerInput(Unit) {
                    awaitPointerEventScope {
                        while (true) {
                            val event = awaitPointerEvent()
                            // 顺带处理鼠标悬停右缘唤出滚动条（本块本就旁观全部事件流；hover 无按压、无人消费）。
                            // 进出沿驱动：进入唤出/恒显，离开交还看门计时淡出。
                            if (event.type == PointerEventType.Move &&
                                event.changes.none { it.pressed } &&
                                event.changes.firstOrNull()?.type == PointerType.Mouse
                            ) {
                                val inZone = event.changes.first().position.x >= size.width - SCROLLBAR_HOT_ZONE.toPx()
                                if (inZone != scrollbarClock.hovering) {
                                    scrollbarClock.hovering = inZone
                                    if (inZone) markUserScroll()
                                }
                            }
                            if (event.type != PointerEventType.Scroll) continue
                            var dx = 0f
                            var dy = 0f
                            event.changes.forEach { dx += it.scrollDelta.x; dy += it.scrollDelta.y }
                            if (dx == 0f && dy == 0f) continue
                            markUserScroll()
                            val step = liveLineHeightPx.value * 3f
                            scrollY = (scrollY + dy * step).coerceIn(0f, liveMaxScrollY.value)
                            scrollX = (scrollX + dx * step).coerceIn(0f, liveMaxScrollX.value)
                            // 滚轮/触控板滚动无触屏/鼠标 down，不会经点按块清菜单——在此显式收起，避免气泡黏在窗口边、
                            // 且让悬浮条 show 门转假、不再订阅滚动（混合触屏+指点设备场景）。
                            showTouchMenu = false
                            showContextMenu = false
                            event.changes.forEach { it.consume() }
                        }
                    }
                }
                .pointerInput(Unit) {
                    // 双指缩放 + 缩放后单指跟手平移（一个连续手势内完成，避免与 scrollable 交接跳变）。≥2 指时累积连续 previewScale
                    // （仅 draw 阶段读 → 零重组/零重排/零重测）并消费；松手一次 commitZoom 换字号 + 重排。提交后若仍剩一指，**本处理器**
                    // 继续按其位移 1:1 平移「已重排」的内容（并消费，scrollable 不再插手）——平移直接作用在提交后真实内容上、跟手、无跳。
                    // 平移不提供 fling（缩放后微调足矣）；抬手结束。纯单指（从未双指）不进任何分支、不消费 → 照常交 scrollable 滚动/惯性。
                    awaitEachGesture {
                        awaitFirstDown(requireUnconsumed = false)
                        var active = false // 已进入双指缩放
                        var committed = false // 本轮已提交（其后剩余单指走跟手平移）
                        var fontStart = fontSizeSp // 手势起始字号（commitZoom 折算基准；scrollX/Y 手势期不变，直接读实时值即 base）
                        var prevCentroid = Offset.Unspecified // 上一帧双指中点（增量变换用）
                        var freeTx = 0f // 未受阻尼的自由平移（增量累积于此）；显示时经 rubber-band 映射写入 previewTx/Ty
                        var freeTy = 0f
                        var panLast = Offset.Unspecified // 提交后剩余单指的上一位置（跟手平移基准）
                        var panActive = false // 跟手平移是否已越过 touchSlop（抬指残留微移 / 触屏 liftoff 抖动不算平移）
                        do {
                            val event = awaitPointerEvent()
                            val pressed = event.changes.count { it.pressed }
                            if (pressed >= 2) {
                                panLast = Offset.Unspecified; panActive = false // 回到双指：清除单指平移基准与激活态
                                // calculateCentroid 只计入「本帧且上帧都按下」的指针：第二指刚落那帧新指 previousPressed=false 不计入 →
                                // 返回的是旧那根手指位置、非真中点。故新指落下帧不拿它当基准，等下一帧两指都稳定按下再确立中点。
                                val c = event.calculateCentroid(useCurrent = true)
                                if (!active) {
                                    // 缩放起始：起始态归零、捕获起始字号。prevCentroid 置未定，等首个稳定双指中点帧再确立。
                                    active = true; committed = false
                                    fontStart = fontSizeSp
                                    previewScale = 1f; previewTx = 0f; previewTy = 0f
                                    freeTx = 0f; freeTy = 0f
                                    prevCentroid = Offset.Unspecified
                                } else if (c != Offset.Unspecified) {
                                    if (prevCentroid == Offset.Unspecified) {
                                        prevCentroid = c // 首个稳定双指中点：仅确立增量基准，本帧不动变换（避免「新落指帧中点=单指」的首帧大跳）
                                    } else {
                                        // 逐帧增量累积到**自由**平移 freeTx/Ty：绕当前中点缩放 ez（撞字号界 ez→1、仍可平移）+ 中点位移 → T'=ez·T+(c−ez·prevC)。
                                        val newScale = ZoomMath.clampScaleToFontRange(
                                            previewScale * event.calculateZoom(),
                                            fontStart,
                                            ZOOM_MIN_SP,
                                            ZOOM_MAX_SP
                                        )
                                        val ez = if (previewScale > 0f) newScale / previewScale else 1f
                                        if (!softWrap) freeTx = ez * freeTx + (c.x - ez * prevCentroid.x) // 换行钉左：freeTx 恒 0
                                        freeTy = ez * freeTy + (c.y - ez * prevCentroid.y)
                                        previewScale = newScale
                                        prevCentroid = c
                                        // 显示 = 把自由平移的等效滚动 eff = s·base − free **硬钳到界内** [0, max@s]：四向跟手、到边硬停、绝不越界
                                        // （放得下的轴 max=0 → 钉死）。自由层 freeTx/Ty 仍纯累积 → 边缘无漂移、回拉即跟。越界量恒 0 ⇒ 提交无越界、松手弹簧不触发。
                                        val vw = viewportWidth
                                        val vh = viewportHeight
                                        if (!softWrap) {
                                            val maxSX = (newScale * liveContentWidth.value - vw).coerceAtLeast(0f)
                                            previewTx = newScale * scrollX - (newScale * scrollX - freeTx).coerceIn(0f, maxSX)
                                        }
                                        // 纵向上界含底部留白（vh 实时读、不 stale）：预览可停在留白里，与提交 re-clamp 的 maxScrollY 同式 ⇒ 起手不 snap、松手不跳。
                                        val maxSY =
                                            (newScale * liveContentHeight.value - vh + vh * BOTTOM_SCROLL_PAD_FRACTION).coerceAtLeast(0f)
                                        previewTy = newScale * scrollY - (newScale * scrollY - freeTy).coerceIn(0f, maxSY)
                                    }
                                }
                                // 双指期**一律消费**（含持距漂移帧）：否则未消费帧落到 scrollable2D、攒 fling，非同时松手时甩出 → 大跳。
                                event.changes.forEach { it.consume() }
                            } else if (active && !committed) {
                                // 掉到 <2 指：立即提交缩放（内部把变换折进 scroll + 预览归位），并以剩余单指为基准接管跟手平移。
                                commitZoomLive.value(fontStart, previewScale, previewTx, previewTy)
                                committed = true; active = false
                                val p = event.changes.firstOrNull { it.pressed }
                                panLast = p?.position ?: Offset.Unspecified
                                // 连同「刚抬起」的那根手指一并消费：否则 scrollable2D 可能在其 up 上结束拖动并 fling → 松手大跳。
                                event.changes.forEach { it.consume() }
                            } else if (committed && pressed >= 1) {
                                // 缩放后剩余单指继续滑动：越过 touchSlop 才算「跟手平移」，据位移 1:1 平移已重排内容（跟手、基于手指下真实内容）。
                                // 未越 slop 前不动：滤掉「本想同时松手却一根先抬」时残留手指的微移 / 触屏 liftoff 抖动，避免误滚动跳变。全程消费防落 scrollable。
                                val p = event.changes.firstOrNull { it.pressed }
                                if (p != null) {
                                    if (panLast != Offset.Unspecified) {
                                        val d = p.position - panLast
                                        if (!panActive) {
                                            // 距 commit 基准累积（未越 slop 前不更新 panLast）；越档后从当前位置起算、丢弃 slop 内位移。
                                            if (d.getDistance() > viewConfiguration.touchSlop) {
                                                panActive = true; panLast = p.position
                                            }
                                        } else {
                                            markUserScroll() // 缩放后单指平移也是用户滚动
                                            scrollX = (scrollX - d.x).coerceIn(0f, liveMaxScrollX.value)
                                            scrollY = (scrollY - d.y).coerceIn(0f, liveMaxScrollY.value)
                                            panLast = p.position
                                        }
                                    } else panLast = p.position
                                    p.consume()
                                }
                            }
                        } while (event.changes.any { it.pressed })
                        // 兜底（双指同时抬起 / 手势取消，未经上面的 <2 指分支）：仍未提交则提交一次（内部折算进 scroll + 预览归位）。
                        if (active && !committed) commitZoomLive.value(fontStart, previewScale, previewTx, previewTy)
                    }
                }
                // caretRectInEditor：光标在内容本地坐标的矩形（复用 caretRectLive 的当前帧几何，含滚动/gutter），
                // 桌面据此把 IME 候选窗定位到光标处；Android 忽略。r = [x, top, bottom]。
                .editorTextInput(
                    engine,
                    enabled = !readOnly,
                    caretRectInEditor = {
                        caretRectLive.value(engine.caret)?.let { r -> Rect(r[0], r[1], r[0] + 1f, r[2]) }
                    },
                )
                .onKeyEvent { ev ->
                    if (ev.type != KeyEventType.KeyDown) return@onKeyEvent false
                    val shift = ev.isShiftPressed
                    // 补全列表优先消费导航与确认键；其他按键继续交给编辑器，并由文档/光标变化刷新候选。
                    if (completionResult != null) {
                        when (ev.key) {
                            Key.Escape -> {
                                completionResult = null
                                return@onKeyEvent true
                            }

                            Key.DirectionUp -> {
                                moveCompletionSelection(-1)
                                return@onKeyEvent true
                            }

                            Key.DirectionDown -> {
                                moveCompletionSelection(1)
                                return@onKeyEvent true
                            }

                            Key.Enter, Key.NumPadEnter, Key.Tab -> {
                                acceptCompletion(completionSelectedIndex)
                                return@onKeyEvent true
                            }

                            else -> Unit
                        }
                    }
                    // 平台各异的剪贴板 / 导航快捷键先经 resolveEditorKeyCommand 归一（mac=Cmd/Opt，其余=Ctrl）。
                    resolveEditorKeyCommand(ev)?.let { cmd ->
                        when (cmd) {
                            // 剪贴/全选/撤销/重做统一走执行器（只读门在其内：复制/全选可用、改文档的动作 no-op）。
                            EditorKeyCommand.SelectAll -> clipboardActions.perform(EditorContextAction.SelectAll)
                            EditorKeyCommand.Copy -> clipboardActions.perform(EditorContextAction.Copy)
                            EditorKeyCommand.Cut -> clipboardActions.perform(EditorContextAction.Cut)
                            EditorKeyCommand.Paste -> clipboardActions.perform(EditorContextAction.Paste)
                            EditorKeyCommand.Undo -> clipboardActions.perform(EditorContextAction.Undo)
                            EditorKeyCommand.Redo -> clipboardActions.perform(EditorContextAction.Redo)

                            // 查找 / 替换 / 跳转行号：打开各自的停靠条（焦点移交输入框），彼此互斥；
                            // F3 / Cmd+G 系在查找条开着时步进导航。
                            EditorKeyCommand.Find -> {
                                gotoSession.close(); findSession.open(withReplace = false)
                            }

                            EditorKeyCommand.Replace -> {
                                gotoSession.close(); findSession.open(withReplace = !readOnlyLive.value)
                            }

                            EditorKeyCommand.FindNext -> if (findSession.visible) findSession.next()
                            EditorKeyCommand.FindPrev -> if (findSession.visible) findSession.prev()
                            EditorKeyCommand.GotoLine -> {
                                findSession.close(); gotoSession.open()
                            }

                            EditorKeyCommand.Completion -> controller.openCompletion()

                            // 注释标记来自当前语言的高亮插件：行注释优先，无行注释的语言（HTML 类）退块注释；
                            // 两者皆无（纯文本）= 静默无操作。
                            EditorKeyCommand.ToggleComment -> if (!readOnlyLive.value) {
                                val linePrefix = resolvedHighlighter?.lineCommentPrefix
                                if (linePrefix != null) engine.toggleLineComment(linePrefix)
                                else resolvedHighlighter?.blockComment?.let { engine.toggleBlockComment(it.open, it.close) }
                            }

                            EditorKeyCommand.WordLeft -> engine.moveCaretByWord(-1, shift)
                            EditorKeyCommand.WordRight -> engine.moveCaretByWord(1, shift)
                            EditorKeyCommand.LineStart -> engine.moveCaretToLineStart(shift)
                            EditorKeyCommand.LineEnd -> engine.moveCaretToLineEnd(shift)
                            EditorKeyCommand.DocStart -> engine.moveCaretToDocStart(shift)
                            EditorKeyCommand.DocEnd -> engine.moveCaretToDocEnd(shift)
                            EditorKeyCommand.PageUp -> engine.movePage(-1, pageLines(), shift)
                            EditorKeyCommand.PageDown -> engine.movePage(1, pageLines(), shift)
                        }
                        return@onKeyEvent true
                    }
                    // 平台无关：纯方向 / 编辑 / 字符键。
                    // composing（预编辑）进行中，方向/回车/退格等键由输入法消费、不回落到此处，故这些处理器
                    // 无需按 composing 设闸；可打印字符路径（insertTypedCharacter）则显式设了 composing 闸。
                    when (ev.key) {
                        // 停靠条开着时 Esc 关闭（条内 Esc 由其字段自行处理；这里接住焦点在编辑器时的 Esc）。
                        Key.Escape -> when {
                            findSession.visible -> {
                                findSession.close(); true
                            }

                            gotoSession.visible -> {
                                gotoSession.close(); true
                            }

                            else -> false
                        }

                        Key.DirectionLeft -> {
                            engine.moveCaretHorizontally(-1, shift); true
                        }

                        Key.DirectionRight -> {
                            engine.moveCaretHorizontally(1, shift); true
                        }

                        Key.DirectionUp -> {
                            if (softWrap) moveCaretVisual(-1, shift) else engine.moveCaretVertically(-1, shift); true
                        }

                        Key.DirectionDown -> {
                            if (softWrap) moveCaretVisual(1, shift) else engine.moveCaretVertically(1, shift); true
                        }

                        Key.Backspace -> {
                            if (!readOnly) engine.backspace(); true
                        }

                        Key.Delete -> {
                            if (!readOnly) engine.deleteForward(); true
                        }

                        Key.Enter, Key.NumPadEnter -> {
                            if (!readOnly) engine.insertNewlineAutoIndent(); true
                        }
                        // 可编辑时 Tab 消费，防止焦点遍历把焦点带走；只读放行焦点切换。
                        // Shift+Tab 反缩进（光标时作用当前行）；有选区的 Tab 块缩进；光标 Tab 插一档缩进。
                        Key.Tab -> if (readOnly) false else {
                            when {
                                shift -> engine.outdentSelectedLines()
                                engine.selStart != engine.selEnd -> engine.indentSelectedLines()
                                else -> engine.insert(EditorEngine.INDENT_UNIT)
                            }
                            true
                        }
                        // 可打印字符回退（桌面无 IME 参与的字符经 KeyEvent 到达；Android 走 InputConnection、返回 false）。
                        else -> insertTypedCharacter(engine, ev, readOnly)
                    }
                }
                .focusRequester(focusRequester)
                .focusable(interactionSource = interaction)
                // 点按手势（只读时同样挂载：仅不弹软键盘、不接受编辑）。自定义而非 detectTapGestures：
                // 后者一旦提供 onDoubleTap，就要等双击超时(~300ms)确认才回调 onTap，点击落光标发闷。这里
                // 第一击抬手「立即」落光标 + 弹键盘；双击窗口内若来第二击，升级为选词（桌面双击、移动端双击皆可）。
                // 长按拖拽由下方 block 接管——指针被其消费/取消时 waitForUpOrCancellation 返回 null，本 block 让位。
                .pointerInput(engine) {
                    awaitEachGesture {
                        val down = awaitFirstDown(requireUnconsumed = false)
                        if (down.isConsumed) return@awaitEachGesture // 滚动条 thumb 已接管（本文件唯一消费 down 的内层块）
                        if (down.type == PointerType.Mouse) return@awaitEachGesture // 仅鼠标让给最内层鼠标块；触控笔/未知指针走此触屏路径（勿漏掉 stylus）
                        completionResult = null
                        lastInteractionWasMouse = false
                        showTouchMenu = false // 新一次触屏按下先隐藏菜单；本次手势结算（落光标/选词）后再亮
                        showContextMenu = false
                        // 等抬手，但包一层长按超时：若超时前未抬手，说明这次已升级为长按（由下方 block 选词），
                        // 本 block 直接让位、不落光标——否则纯长按选词后「不拖动直接抬手」会把刚选好的词塌成光标
                        // （长按 block 不移动时不消费任何 change，waitForUpOrCancellation 拿到未消费的 up 会误触发）。
                        val up = withTimeoutOrNull(viewConfiguration.longPressTimeoutMillis) {
                            waitForUpOrCancellation()
                        } ?: return@awaitEachGesture
                        val tapPos = positionAtLive.value(up.position)
                        // 移动端惯例：一点定位、再点同处才唤菜单。仅「点在已有光标处」（空选区且位置不变）才亮粘贴气泡；
                        // 首次点按或点到别处只落/移光标、不弹菜单。双击选词与长按选区仍立即出工具条（见下方 / drag-settle）。
                        val onExistingCaret = engine.selection.isEmpty && engine.caret == tapPos
                        engine.setCursor(tapPos)
                        focusRequester.requestFocus()
                        if (!readOnlyLive.value) {
                            engine.requestShowKeyboard?.invoke()
                            pingCaretHandle() // 落光标后显示泪滴手柄（4s 后自动隐藏）
                        }
                        showTouchMenu = onExistingCaret
                        val second = withTimeoutOrNull(viewConfiguration.doubleTapTimeoutMillis) {
                            awaitFirstDown(requireUnconsumed = false)
                        }
                        val up2 = if (second != null) waitForUpOrCancellation() else null
                        if (up2 != null) {
                            val pos2 = positionAtLive.value(up2.position)
                            // 双击选词【有容错】：两击落点在 touchSlop 内即算双击、选词并出选区工具条（与鼠标多击的就近判定同阈值）。
                            // 而气泡菜单【无容错】：只在精确点回原光标处才出（上面的 onExistingCaret），此处第二击一旦移开就只挪光标、不出菜单。
                            if ((up2.position - up.position).getDistance() <= viewConfiguration.touchSlop) {
                                val w = engine.wordRangeAt(pos2)
                                engine.setSelection(w.start, w.end)
                                focusRequester.requestFocus()
                                showTouchMenu = true
                            } else {
                                engine.setCursor(pos2)
                                focusRequester.requestFocus()
                                showTouchMenu = false
                            }
                        }
                    }
                }
                .pointerInput(engine) {
                    // 长按才进入选择：长按处先选中该词，随后拖拽扩展选区。普通拖拽不在此消费，
                    // 于是落到上面的 scrollable 去滚动页面（移动端标准行为）。拖到上下边缘由
                    // selectionDragPos 驱动的边缘自动滚动 effect 接管。只读模式同样可用（纯选择、不改文档）。
                    detectDragGesturesAfterLongPress(
                        onDragStart = { p ->
                            // 鼠标静止长按也会触发本回调（awaitLongPressOrCancellation 不校验初始 down 的消费，
                            // 无后续移动即超时触发）；此时 lastInteractionWasMouse 已被最内层鼠标块置 true，跳过——
                            // 否则会给鼠标选区错误选词并重新露出触屏手柄。触屏长按时该标记已由点按块置 false。
                            // 抓住滚动条 thumb 静止超过长按超时同理触发到此，靠 dragging 标志显式互斥——
                            // 消费 down 挡不住计时器型检测器。
                            if (!lastInteractionWasMouse && !scrollbarDragging.value) {
                                haptic.performHapticFeedback(HapticFeedbackType.LongPress) // 长按进入选择时轻震确认
                                val w = engine.wordRangeAt(positionAtLive.value(p))
                                engine.setSelection(w.start, w.end)
                                // 锚定整个初选词、按词粒度扩展（不用按下点作字符锚）——否则边缘自动滚动或首次拖拽会把
                                // 刚选好的词塌成字符级光标（长按靠近视口边缘时尤其明显：一选中就瞬间消失）。
                                selectionWordAnchor = w
                                selectionAnchor = null
                                focusRequester.requestFocus()
                                selectionDragPos = p
                                selectionDragActive = true
                            }
                        },
                        onDrag = { change, _ ->
                            selectionDragPos = change.position
                            selectionWordAnchor?.let { engine.selectWordRange(it, positionAtLive.value(change.position)) }
                        },
                        onDragEnd = { selectionDragPos = null; selectionDragActive = false; selectionWordAnchor = null },
                        onDragCancel = { selectionDragPos = null; selectionDragActive = false; selectionWordAnchor = null },
                    )
                }
                // 手柄拖拽：抓到光标/选区端点手柄即接管——重定位光标或调整选区端点；未抓到则不消费，让位给
                // 滚动/点按/长按。触屏专用（鼠标块在其下方、更内层，鼠标 down 会被其消费；本块 Main 阶段仍先于
                // scrollable 抢占触屏手柄拖拽）；命中盒经 caretRectLive 取当前帧几何，避开 stale-capture 坑。
                .pointerInput(engine) {
                    awaitEachGesture {
                        val down = awaitFirstDown(requireUnconsumed = false)
                        if (down.type == PointerType.Mouse) return@awaitEachGesture // 仅鼠标让给最内层鼠标块；触控笔/未知指针可抓手柄
                        val p = down.position
                        val sel = engine.selection
                        fun hit(kind: HandleKind, at: TextPosition): Boolean {
                            val r = caretRectLive.value(at) ?: return false
                            return EditorGeometry.handleGeometry(kind, r[0], r[1], r[2], handleRadiusPx, handleSlopPx)
                                .hitContains(p.x, p.y)
                        }
                        // 有选区时只看两端手柄（终点优先）；无选区且光标手柄可见、非只读时看光标手柄。
                        var kind: HandleKind? = null
                        var anchor: TextPosition? = null
                        var grabbed: TextPosition? = null
                        if (!sel.isEmpty) {
                            when {
                                hit(HandleKind.SelectionEnd, sel.end) -> {
                                    kind = HandleKind.SelectionEnd; anchor = sel.start; grabbed = sel.end
                                }

                                hit(HandleKind.SelectionStart, sel.start) -> {
                                    kind = HandleKind.SelectionStart; anchor = sel.end; grabbed = sel.start
                                }
                            }
                        } else if (caretHandleVisible && !readOnlyLive.value && hit(HandleKind.Caret, sel.start)) {
                            kind = HandleKind.Caret; grabbed = sel.start
                        }
                        if (kind == null) return@awaitEachGesture // 未抓到手柄，让位给滚动/点按/长按

                        down.consume()
                        lastInteractionWasMouse = false // 触屏/触控笔抓手柄：确保手柄可见（此前若为鼠标选区、手柄被抑制）
                        haptic.performHapticFeedback(HapticFeedbackType.TextHandleMove) // 抓住手柄时轻震确认
                        // 纵向抓取偏移：目标点落在光标中部而非手指处（手指压在泪滴上、光标在其上方）。
                        val gr = grabbed?.let { caretRectLive.value(it) }
                        val grabDy = if (gr != null) (gr[1] + gr[2]) / 2f - p.y else 0f
                        fun mapped(o: Offset): TextPosition = positionAtLive.value(Offset(o.x, o.y + grabDy))

                        if (kind == HandleKind.Caret) {
                            engine.setCursor(mapped(p)); pingCaretHandle()
                            // 先等真正拖动（越过 touchSlop）才进入重定位 + 边缘自动滚动；纯点按（按下即抬起、位移不过 slop）
                            // 只落光标、绝不激活自动滚动——否则光标本就靠边时，一按到边缘热区就被 caretDrag effect 当成「停在边缘」
                            // 带着滚。以位移阈值区分：点=只落光标永不滚，拖=才滚，且自动滚是「拖拽进行时」的属性。
                            // caretDragPos 存已含 grabDy 的目标点（与 mapped 同源），供 effect 判热区并逐帧落光标、可拖到视口外内容。
                            val slop = awaitTouchSlopOrCancellation(down.id) { c, _ -> c.consume() }
                            if (slop != null) {
                                // 拖拽中每落到「新字符/行」补一次轻震（按位置去抖、非逐帧——一帧多次
                                // 位移都在同一字符时不重复震）。比较落定后的 engine.caret（已 clamp）而非原始映射点：拖过
                                // 短行行尾时 caret 停在行尾、原始列还在涨，比 caret 才不会空震。lastHaptic 初值为抓取帧的光标。
                                var lastHaptic = engine.caret
                                caretDragPos = Offset(slop.position.x, slop.position.y + grabDy)
                                engine.setCursor(mapped(slop.position))
                                if (engine.caret != lastHaptic) {
                                    haptic.performHapticFeedback(HapticFeedbackType.TextHandleMove); lastHaptic = engine.caret
                                }
                                caretDragActive = true
                                handleDragActive = true
                                drag(down.id) { change ->
                                    caretDragPos = Offset(change.position.x, change.position.y + grabDy)
                                    engine.setCursor(mapped(change.position))
                                    if (engine.caret != lastHaptic) {
                                        haptic.performHapticFeedback(HapticFeedbackType.TextHandleMove); lastHaptic = engine.caret
                                    }
                                    change.consume()
                                }
                                caretDragActive = false
                                handleDragActive = false
                                caretDragPos = null
                            } else {
                                // 纯点按光标手柄 = 点在光标原位，语义与点按块的 onExistingCaret 完全一致，
                                // 行为也须一致：唤键盘 + 亮粘贴气泡。本块吃掉 down 后点按块整个不跑（含它的
                                // 唤起与气泡分支），且每次点击都续手柄 4s 计时——不在这补齐的话，手指落在
                                // 泪滴上的「点原位」永远既不弹键盘也不出操作栏。拖拽结束不做：拖是调整位置。
                                if (!readOnlyLive.value) engine.requestShowKeyboard?.invoke()
                                showTouchMenu = true
                            }
                            pingCaretHandle() // 抬手后重置 4s 计时
                        } else {
                            val a = anchor!!
                            // 与光标手柄同理：越过 touchSlop 才算拖拽端点、激活延伸 + 边缘自动滚动；纯点按端点手柄（不过 slop）
                            // 保持选区原样、不滚动——否则端点手柄本就靠边时，一按到边缘热区就被选区 effect 当成「停在边缘」带着滚。
                            val slop = awaitTouchSlopOrCancellation(down.id) { c, _ -> c.consume() }
                            if (slop != null) {
                                selectionAnchor = a
                                selectionWordAnchor = null // 端点拖拽走字符级锚
                                // selectionDragPos 存已含 grabDy 的目标点（与 mapped/setSelection 同源）：否则边缘自动滚动 effect 会按
                                // 手柄泪滴（在光标下方约 1.5 行）所在行做「按行闸停」，落到下方短行 → 误判行尾、横向不滚。与光标手柄一致。
                                selectionDragPos = Offset(slop.position.x, slop.position.y + grabDy)
                                selectionDragActive = true
                                handleDragActive = true
                                // 端点每落到「新字符/行」补一次轻震，与光标手柄一致；比较落定后的活动端 engine.caret
                                // （setSelection 的 head，已 clamp）。lastHaptic 初值为抓取的端点位置。
                                var lastHaptic = grabbed
                                engine.setSelection(a, mapped(slop.position))
                                if (engine.caret != lastHaptic) {
                                    haptic.performHapticFeedback(HapticFeedbackType.TextHandleMove); lastHaptic = engine.caret
                                }
                                drag(down.id) { change ->
                                    selectionDragPos = Offset(change.position.x, change.position.y + grabDy)
                                    engine.setSelection(a, mapped(change.position))
                                    if (engine.caret != lastHaptic) {
                                        haptic.performHapticFeedback(HapticFeedbackType.TextHandleMove); lastHaptic = engine.caret
                                    }
                                    change.consume()
                                }
                                selectionDragPos = null
                                selectionDragActive = false
                                handleDragActive = false
                            }
                        }
                    }
                }
                // 鼠标文本交互（最内层：Main 阶段最先处理并消费鼠标 down，使上方触屏块无从触发）。仅 PointerType.Mouse 生效：
                // 单击落光标、Shift+单击延伸选区、双击选词、三击选行；拖拽越过 touchSlop 即按锚粒度框选（字符/词/行），边缘由
                // selectionDragPos/Active 驱动的自动滚动 effect 露出屏外内容。鼠标拖拽不滚动（scrollable2D 本就忽略鼠标拖）。触屏
                // down 在此不消费、直接 return，交回上方触屏块（点按/长按/手柄）——不干扰既有触屏手势。
                .pointerInput(engine) {
                    // 点击计数状态：驻留于本 pointerInput 协程、跨手势保留（awaitEachGesture 在同一协程内循环）。
                    var lastClickTime = 0L
                    var lastClickPos = Offset.Zero
                    var clickCount = 0
                    awaitEachGesture {
                        val down = awaitFirstDown(requireUnconsumed = false)
                        if (down.isConsumed) return@awaitEachGesture // 滚动条 thumb 已接管：down 帧的 setCursor/清选区副作用一并跳过
                        if (down.type != PointerType.Mouse) return@awaitEachGesture // 触屏交给上方触屏块
                        down.consume()
                        completionResult = null
                        lastInteractionWasMouse = true
                        showTouchMenu = false // 切到鼠标：收起触屏悬浮条（其 show 门也含 !lastInteractionWasMouse，这里同时清闩）
                        // keyboardModifiers 携带按下时的 Shift 态；未上报时 shift=false，自然退化为普通单击。
                        val shift = currentEvent.keyboardModifiers.isKeyboardShiftPressed
                        val downPos = down.position
                        val clickPos = positionAtLive.value(downPos)
                        // 右键（副键）由下方专用 raw-event 块处理：桌面(skiko)上「只按副键」不置 pointer.pressed，
                        // awaitFirstDown 根本不触发（故本块在桌面收不到右键）；Android 上会触发，这里也一并让开、不落左键逻辑。
                        if (currentEvent.buttons.isSecondaryPressed) return@awaitEachGesture
                        focusRequester.requestFocus()
                        showContextMenu = false // 左键（或其他）先关右键菜单

                        // 点击计数：与上次点击「时间差 ≤ 双击超时」且「位移 ≤ touchSlop」则累加，否则归 1；>3 回绕到 1。
                        val now = down.uptimeMillis
                        val within = (now - lastClickTime) <= viewConfiguration.doubleTapTimeoutMillis &&
                                (downPos - lastClickPos).getDistance() <= viewConfiguration.touchSlop
                        clickCount = if (within) clickCount + 1 else 1
                        if (clickCount > 3) clickCount = 1
                        lastClickTime = now
                        lastClickPos = downPos

                        // 拖拽锚（互斥）：字符锚=单击 / Shift+单击、词锚=双击、行锚=三击。拖拽与边缘自动滚动据此统一延伸。
                        var charAnchor: TextPosition? = null
                        var wordAnchor: TextRange? = null
                        var lineAnchor: Int? = null
                        when {
                            // Shift+单击：保持原锚不动、head 延伸到点击处；拖拽以「当前选区固定端」为字符锚续延伸。
                            shift -> {
                                val fixed =
                                    if (engine.caret == engine.selection.end) engine.selection.start else engine.selection.end
                                engine.setSelection(fixed, clickPos)
                                charAnchor = fixed
                            }

                            clickCount >= 3 -> {
                                val line = clickPos.line
                                engine.setSelection(TextPosition(line, 0), TextPosition(line, engine.buffer.lineLength(line)))
                                lineAnchor = line
                            }

                            clickCount == 2 -> {
                                val w = engine.wordRangeAt(clickPos)
                                engine.setSelection(w.start, w.end)
                                wordAnchor = w
                            }

                            else -> {
                                engine.setCursor(clickPos)
                                charAnchor = clickPos
                            }
                        }

                        // 越过 touchSlop 才进入框选；纯单击（不过 slop）到此结束，仅落光标 / 选词 / 选行。
                        val slop = awaitTouchSlopOrCancellation(down.id) { c, _ -> c.consume() }
                        if (slop != null) {
                            val cAnchor = charAnchor // 提到 val，供闭包 smart-cast
                            val wAnchor = wordAnchor
                            val lAnchor = lineAnchor
                            fun extendTo(o: Offset) {
                                val c = positionAtLive.value(o)
                                when {
                                    wAnchor != null -> engine.selectWordRange(wAnchor, c)
                                    lAnchor != null -> {
                                        val a = minOf(lAnchor, c.line)
                                        val b = maxOf(lAnchor, c.line)
                                        engine.setSelection(TextPosition(a, 0), TextPosition(b, engine.buffer.lineLength(b)))
                                    }

                                    else -> engine.setSelection(cAnchor ?: c, c)
                                }
                            }
                            // 先把锚交给边缘自动滚动 effect（据此判字符/词/行粒度），再翻起 selectionDragActive 启动它。
                            selectionAnchor = cAnchor
                            selectionWordAnchor = wAnchor
                            selectionLineAnchor = lAnchor
                            selectionDragPos = slop.position
                            selectionDragActive = true
                            extendTo(slop.position)
                            drag(down.id) { change ->
                                selectionDragPos = change.position
                                extendTo(change.position)
                                change.consume()
                            }
                            selectionDragPos = null
                            selectionDragActive = false
                            selectionAnchor = null
                            selectionWordAnchor = null
                            selectionLineAnchor = null
                        }
                    }
                }
                // 右键上下文菜单（跨平台）：桌面(skiko)上「只按副键」不会置 pointer.pressed，awaitFirstDown/detectTap 都收不到，
                // 故上面基于 awaitFirstDown 的鼠标块在桌面漏掉右键（Android 上副键会置 pressed，故那边正常）。这里改用原始事件、
                // 按 isSecondaryPressed 的升/降沿识别（两平台统一）：按下沿落光标（选区外）、抬起沿才弹菜单（press 已收尾，
                // focusable Popup 不会被这次按键的收尾立即关掉）。
                .pointerInput(engine) {
                    awaitPointerEventScope {
                        var wasSecondary = false
                        var anchor = Offset.Zero
                        while (true) {
                            val e = awaitPointerEvent()
                            val ch = e.changes.firstOrNull()
                            if (ch == null || ch.type != PointerType.Mouse) {
                                wasSecondary = false
                                continue
                            }
                            val isSecondary = e.buttons.isSecondaryPressed
                            if (isSecondary && !wasSecondary) {
                                // 副键按下沿：落光标（点在选区外）+ 记锚，消费本次事件。
                                e.changes.forEach { it.consume() }
                                completionResult = null
                                lastInteractionWasMouse = true
                                showTouchMenu = false
                                anchor = ch.position
                                val p = positionAtLive.value(ch.position)
                                val sel = engine.selection
                                if (sel.isEmpty || p < sel.start || p > sel.end) engine.setCursor(p)
                            } else if (!isSecondary && wasSecondary) {
                                // 副键抬起沿：此刻弹菜单。
                                e.changes.forEach { it.consume() }
                                contextMenuAnchor = anchor
                                showContextMenu = true
                            }
                            wasSecondary = isSecondary
                        }
                    }
                }
                // 滚动条 thumb 拖拽（链尾最内：Main pass 最先收 down；未命中不消费、完全透传）。可抓门层层叠：
                // 显示中且非淡出（不留「看不见但可抓」的隐形拦截窗）、滚动静默 ≥150ms（fling 中沿右缘起手的下一次
                // flick 不被绝对映射吃成半个文档的跳转，也避免 fling 循环与本循环同帧竞写 scrollY）、右缘热区命中
                // thumb ±余量、鼠标须主键（右键留给上下文菜单块）、可见泪滴手柄不占。
                .pointerInput(engine) {
                    awaitEachGesture {
                        val down = awaitFirstDown(requireUnconsumed = false)
                        if (!scrollbarClock.shown || scrollbarClock.fading) return@awaitEachGesture
                        if (scrollbarClock.last.elapsedNow() < SCROLLBAR_QUIET_GRAB_MS.milliseconds) return@awaitEachGesture
                        val vh = viewportHeight
                        val maxY = liveMaxScrollY.value
                        val thumbH = ScrollbarMath.thumbHeight(vh, maxY, SCROLLBAR_MIN_THUMB.toPx())
                        if (thumbH <= 0f) return@awaitEachGesture
                        val thumbTop = ScrollbarMath.thumbTop(vh, maxY, thumbH, scrollY)
                        val hit = ScrollbarMath.hitThumb(
                            down.position.x, down.position.y,
                            viewportWidth, SCROLLBAR_HOT_ZONE.toPx(), thumbTop, thumbH, SCROLLBAR_GRAB_SLACK.toPx(),
                        )
                        if (!hit) return@awaitEachGesture
                        if (down.type == PointerType.Mouse && !currentEvent.buttons.isPrimaryPressed) return@awaitEachGesture
                        if (hitsVisibleTouchHandle(down.position)) return@awaitEachGesture
                        down.consume()
                        scrollbarDragging.value = true // 长按块以此互斥：消费 down 挡不住计时器型检测器
                        scrollbarThumbTopOverride.floatValue = thumbTop
                        val grabOffset = down.position.y - thumbTop
                        // 行空间反解：抓取行锚定 + 轨道分数位移 × 行数。softWrap 的估算内容高会在拖入新区域时
                        // 被可见测量逐帧抬高——像素反解会让 thumb 从指下溜走、拖到轨道底也够不到真实文末；
                        // 行数是唯一稳定量（见 ScrollbarMath.dragTargetLine）。thumb 绘制位置拖拽期直接跟手
                        //（Override），抬手后再按比例回落。
                        val grabLine = lineAtPxLive.value(scrollY)
                        val downY = down.position.y
                        var dragStarted = false
                        fun applyDrag(y: Float) {
                            val target = ScrollbarMath.dragTargetLine(grabLine, downY, y, vh, thumbH, liveLineCount.value)
                            scrollY = lineTopPxLive.value(target).coerceIn(0f, liveMaxScrollY.value)
                            scrollbarThumbTopOverride.floatValue = (y - grabOffset).coerceIn(0f, vh - thumbH)
                            scrollbarClock.last = TimeSource.Monotonic.markNow()
                        }
                        while (true) {
                            val ev = awaitPointerEvent()
                            if (ev.changes.count { it.pressed } >= 2) break // 第二指落下：整手势让给缩放块，避免同帧双写 scrollY
                            val ch = ev.changes.firstOrNull { it.id == down.id } ?: break
                            if (!ch.pressed) {
                                ch.consume() // up 也消费：快抓快放不被点按块当 tap 落光标
                                break
                            }
                            // 抓住即接管：每个 Move 立即消费，slop 判定自做（只看纵向位移——横向偏出热区不中断，
                            // 抓住的拖拽以松手为终点、不以停留区域为条件）。不能用「slop 前不消费」的等待式检测：
                            // 按住 thumb 超过长按超时后，长按检测器的内部状态机会进入拖拽段开始消费 Move（互斥门
                            // 只挡得住它的动作、挡不住消费），等待式 slop 一见被消费的 Move 就取消——「按住变粗后
                            // 下滑没反应」。slop 之前仍不写文档：240k 行下 thumb 1px ≈ 上百行，按压抖动直写会随机跳。
                            if (!dragStarted && abs(ch.position.y - downY) > viewConfiguration.touchSlop) dragStarted = true
                            if (dragStarted) applyDrag(ch.position.y)
                            ch.consume()
                        }
                        scrollbarDragging.value = false
                        scrollbarThumbTopOverride.floatValue = -1f
                        markUserScroll() // 抬手重新武装淡出计时
                    }
                }
                // I 形文本光标：桌面 / 带鼠标的 Android 悬停在文本区时显示；触屏无悬停、无副作用。
                .pointerHoverIcon(PointerIcon.Text)
        ) {
            EditorCanvas(
                engine = engine,
                colors = colors,
                textMeasurer = measurer,
                textStyle = textStyle,
                numberStyle = numberStyle,
                lineHeightPx = lineHeightPx,
                gutterWidthPx = gutterWidthPx,
                padXPx = padXPx,
                lineNumberMode = lineNumberMode,
                scrollX = { scrollX.coerceIn(0f, maxScrollX) },
                scrollY = { scrollY.coerceIn(0f, maxScrollY) },
                // 与 scrollY/scrollX 传参同源钳制：maxScroll 骤减那一帧（底部捏合放大 / 收起 IME），行锚与像素
                // 偏移都读钳制后的 scrollY，二者恒定锚同一窗口，不再顶部留一帧空白（re-clamp effect 下一帧才生效）。
                // 入参 extraTopPx 为缩放预览的可见带上界（预缩放屏幕 y）：非缩放时为 0（等同原状），缩小时为负、
                // 令起始行前移、补齐外扩露出的行。
                firstVisibleLine = { extraTopPx -> (lineAtPx(scrollY.coerceIn(0f, maxScrollY) + extraTopPx) - 3).coerceAtLeast(0) },
                lineTopPx = ::lineTopPx,
                refBaselinePx = refBaselinePx,
                // 光标手柄跟随光标线的可见性：编辑器失焦（如查找框输入中）光标不画，手柄也不悬空。
                caretHandleVisible = { caretHandleVisible && !readOnly && !lastInteractionWasMouse && editorFocused.value },
                selectionHandlesVisible = { !lastInteractionWasMouse },
                handleRadiusPx = handleRadiusPx,
                layoutFor = ::layoutFor,
                charW = charWpx,
                isGridLine = ::isGridLine,
                gridRefBaseline = gridRefBaseline,
                gridRefCursorTop = gridRefCursor.top,
                gridRefCursorBottom = gridRefCursor.bottom,
                // 查找命中高亮（draw 阶段读 session 的行索引表与当前命中下标，重算即重绘、不重组）。
                findSpansForLine = findSession::spansForLine,
                activeFindIndex = { findSession.activeIndex },
                bracketMatch = { bracketMatchState.value },
                highlightCache = highlightCache,
                // 缩放预览的运行态仿射（draw 阶段读）：previewScale 连续缩放系数、previewTx/previewTy 两轴平移（屏幕 px）。手势逐帧按
                // 「绕当前双指中点缩放 + 中点位移平移」增量累积 → 四向自由跟手；换行下 previewTx 恒 0（正文/gutter 钉左）。恒 (1,0,0) 逐像素等价原状。
                previewScale = { previewScale },
                previewTx = { previewTx },
                previewTy = { previewTy },
                modifier = Modifier.fillMaxSize(),
            )
            // 光标独立图层：叠在正文之上，blink 只切本层 alpha、不重放正文画布（P10）。
            CursorOverlay(
                engine = engine,
                colors = colors,
                // 持焦才画：查找框等部件夺焦时正文不再闪光标（同一时刻只有焦点所在处有插入光标）。
                caretVisible = { !readOnly && blink && editorFocused.value },
                scrollX = { scrollX.coerceIn(0f, maxScrollX) },
                scrollY = { scrollY.coerceIn(0f, maxScrollY) },
                lineTopPx = ::lineTopPx,
                refBaselinePx = refBaselinePx,
                layoutFor = ::layoutFor,
                charW = charWpx,
                isGridLine = ::isGridLine,
                gridRefBaseline = gridRefBaseline,
                gridRefCursorTop = gridRefCursor.top,
                gridRefCursorBottom = gridRefCursor.bottom,
                gutterWidthPx = gutterWidthPx,
                padXPx = padXPx,
                previewScale = { previewScale }, // 缩放预览期不画光标（正文在缩放、光标会脱节）
                modifier = Modifier.fillMaxSize(),
            )
            // 滚动条独立图层：淡入淡出只重绘本小层，不整层重录主画布（同 CursorOverlay 的隔离理由）。
            ScrollbarOverlay(
                colors = colors,
                alpha = { scrollbarAlpha.value },
                dragging = { scrollbarDragging.value },
                thumbTopOverridePx = { scrollbarThumbTopOverride.floatValue },
                scrollY = { scrollY.coerceIn(0f, maxScrollY) },
                maxScrollY = { liveMaxScrollY.value },
                minThumbPx = with(density) { SCROLLBAR_MIN_THUMB.toPx() },
                modifier = Modifier.fillMaxSize(),
            )
            // 放大镜：宿主在窗口级 Popup（可浮到编辑器上方的工具栏/状态栏空间），仅光标/选区端点手柄拖拽时（handleDragActive）出现。
            MagnifierOverlay(
                engine = engine,
                colors = colors,
                active = { handleDragActive },
                // 连续手指位置（逐帧写）：光标手柄用 caretDragPos、选区端点用 selectionDragPos；胶囊据其 x 平滑跟手不跳。
                dragPos = { caretDragPos ?: selectionDragPos },
                caretVisible = { !readOnly && blink && editorFocused.value }, // 镜内光标随主编辑器同一 blink 闪烁、同一焦点门
                viewportWidth = { viewportWidth },     // 水平钳制到视口
                contentTopInWindow = { contentTopInWindow }, // 允许胶囊上浮到窗口顶附近
                scrollX = { scrollX.coerceIn(0f, maxScrollX) },
                scrollY = { scrollY.coerceIn(0f, maxScrollY) },
                lineTopPx = ::lineTopPx,
                lineHeightPx = lineHeightPx,
                refBaselinePx = refBaselinePx,
                layoutFor = ::layoutFor,
                textMeasurer = measurer,
                textStyle = textStyle,
                numberStyle = numberStyle,
                charW = charWpx,
                isGridLine = ::isGridLine,
                gridRefBaseline = gridRefBaseline,
                gridRefCursorTop = gridRefCursor.top,
                gridRefCursorBottom = gridRefCursor.bottom,
                gutterWidthPx = gutterWidthPx,
                padXPx = padXPx,
                highlightCache = highlightCache,
            )
            completionResult?.let { result ->
                CompletionPopup(
                    result = result,
                    selectedIndex = completionSelectedIndex,
                    caret = engine.caret,
                    colors = colors,
                    posRect = { caretRectLive.value(it) },
                    onSelect = { completionSelectedIndex = it },
                    onAccept = acceptCompletion,
                )
            }
            // 触屏文本操作悬浮条（选区上方浮条 / 点光标粘贴气泡）。show 门：已触发 && 非鼠标 && 未在拖手柄/框选；
            // 宿主在本 Box 内，Popup 锚定本 Box → posRect 的编辑器局部坐标即定位基准。
            SelectionActionToolbar(
                engine = engine,
                colors = colors,
                show = {
                    completionResult == null && showTouchMenu && !lastInteractionWasMouse &&
                        !handleDragActive && !selectionDragActive
                },
                readOnly = readOnly,
                posRect = { caretRectLive.value(it) },
                onPerform = { clipboardActions.perform(it); showTouchMenu = false },
            )
            // 桌面右键上下文菜单：锚在右键按下点（编辑器局部）。
            EditorContextMenu(
                engine = engine,
                colors = colors,
                readOnly = readOnly,
                show = { showContextMenu },
                anchor = { contextMenuAnchor },
                onDismiss = { showContextMenu = false },
                onPerform = { clipboardActions.perform(it); showContextMenu = false },
            )
            }
            if (showEndSymbolBar) {
                VerticalSymbolBar(
                    symbols = symbols,
                    colors = colors,
                    onSymbol = { symbol -> engine.typeCharacter(symbol.value) },
                    windowInsets = bottomBarInsets.takeUnless { handlesBottomInsets },
                )
            }
        }
        // 底部符号快捷条：只读不显示。点键只把 value 交 engine 插到光标处，**不主动聚焦/弹键盘**：
        // 键用 detectTapGestures 不抢焦点——键盘开着时点键焦点不丢、IME 会话不销毁、键盘照旧不收；
        // 键盘没开时点键也就不会意外唤起输入法（仅把符号落到光标）。insets 交给它自管底部沉浸。
        if (showBottomSymbolBar) {
            SymbolBar(
                symbols = symbols,
                colors = colors,
                onSymbol = { symbol -> engine.typeCharacter(symbol.value) }, // 键入原语：{ [ ( " 等也自动配对；Tab 多字符自动退化为原样插入
                windowInsets = bottomBarInsets,
            )
        }
        if (bottomBar != null) {
            EditorBottomBar(
                colors = colors,
                windowInsets = bottomBarInsets,
            ) {
                bottomBar(colors)
            }
        }
    }
}

/**
 * 选区拖拽的边缘自动滚动（单维度，纵横通用）：拖拽点 [pos] 进入近端/远端「热区」时返回每帧滚动步长
 * （近端为负、远端为正），步长随进入热区的深度（含越过边缘）线性放大，上限 3 倍。不在热区返回 0。
 * [extent] 为该维度视口尺寸，[unit] 为步长基准像素（用行高即可）。
 */
private fun edgeAutoScrollSpeed(pos: Float, extent: Float, unit: Float): Float {
    if (extent <= 0f) return 0f
    val hot = unit * 2.5f
    val maxStep = unit * 0.6f
    return when {
        pos < hot -> -maxStep * ((hot - pos) / hot).coerceIn(0f, 3f)
        pos > extent - hot -> maxStep * ((pos - (extent - hot)) / hot).coerceIn(0f, 3f)
        else -> 0f
    }
}
