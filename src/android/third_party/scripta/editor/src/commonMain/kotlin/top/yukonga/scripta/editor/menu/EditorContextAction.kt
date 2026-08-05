package top.yukonga.scripta.editor.menu

import androidx.compose.ui.unit.IntOffset
import kotlin.math.roundToInt

/** 剪贴 / 选择 / 历史的语义操作。触屏悬浮条与桌面右键菜单共用同一套动作与执行器。 */
enum class EditorContextAction { Undo, Redo, Cut, Copy, Paste, SelectAll }

/** 中文显示名（宿主 Mishka 为中文界面；如需本地化可后续经参数外露）。 */
val EditorContextAction.zhLabel: String
    get() = when (this) {
        EditorContextAction.Undo -> "撤销"
        EditorContextAction.Redo -> "重做"
        EditorContextAction.Cut -> "剪切"
        EditorContextAction.Copy -> "复制"
        EditorContextAction.Paste -> "粘贴"
        EditorContextAction.SelectAll -> "全选"
    }

/**
 * 各操作在当前编辑器状态下是否「适用」。纯数据，供两种呈现各自决定隐藏（触屏悬浮条）或置灰（桌面右键）。
 *
 * - [cut]：可编辑且有非空选区。
 * - [copy]：有非空选区（只读也能复制）。
 * - [paste]：可编辑即可（剪贴板是否有文本要 suspend 读，不在此判定——空剪贴板粘贴自然 no-op，与硬键盘一致）。
 * - [selectAll]：文档非空且尚未全选。
 * - [undo] / [redo]：可编辑且对应历史栈非空。
 */
data class ContextActionAvailability(
    val cut: Boolean,
    val copy: Boolean,
    val paste: Boolean,
    val selectAll: Boolean,
    val undo: Boolean = false,
    val redo: Boolean = false,
) {
    fun isAvailable(action: EditorContextAction): Boolean = when (action) {
        EditorContextAction.Undo -> undo
        EditorContextAction.Redo -> redo
        EditorContextAction.Cut -> cut
        EditorContextAction.Copy -> copy
        EditorContextAction.Paste -> paste
        EditorContextAction.SelectAll -> selectAll
    }
}

fun contextActionAvailability(
    readOnly: Boolean,
    hasSelection: Boolean,
    allSelected: Boolean,
    docNonEmpty: Boolean,
    canUndo: Boolean = false,
    canRedo: Boolean = false,
): ContextActionAvailability = ContextActionAvailability(
    cut = !readOnly && hasSelection,
    copy = hasSelection,
    paste = !readOnly,
    selectAll = docNonEmpty && !allSelected,
    undo = !readOnly && canUndo,
    redo = !readOnly && canRedo,
)

/**
 * 触屏悬浮条按序要显示的操作（只保留适用项）。
 * 有选区 → 选区工具条候选 [Cut, Copy, Paste, SelectAll]；无选区 → 粘贴气泡候选 [Paste, SelectAll]。
 * 全不适用时返回空列表（调用方据此不显示）。
 */
fun visibleTouchActions(
    availability: ContextActionAvailability,
    hasSelection: Boolean,
): List<EditorContextAction> {
    val order = if (hasSelection) {
        listOf(
            EditorContextAction.Cut, EditorContextAction.Copy,
            EditorContextAction.Paste, EditorContextAction.SelectAll,
        )
    } else {
        listOf(EditorContextAction.Paste, EditorContextAction.SelectAll)
    }
    return order.filter { availability.isAvailable(it) }
}

/**
 * 触屏悬浮条在窗口内的左上角落点（像素）。围绕锚点水平居中于 [anchorCenterX]，竖直优先放在 [anchorTopY] 上方
 * 留 [gap]；上方越过 [margin]（贴到窗口顶）时翻到 [anchorBottomY] 下方。最终钳进窗口 [ [margin], size − margin ] 内。
 * 纯函数（入参均为窗口坐标像素），便于单测翻转/钳制。
 */
fun toolbarTopLeft(
    anchorCenterX: Float,
    anchorTopY: Float,
    anchorBottomY: Float,
    contentW: Int,
    contentH: Int,
    windowW: Int,
    windowH: Int,
    gap: Int,
    margin: Int,
): IntOffset {
    val x = (anchorCenterX - contentW / 2f).roundToInt()
        .coerceIn(margin, (windowW - contentW - margin).coerceAtLeast(margin))
    var y = (anchorTopY - gap - contentH).roundToInt()
    if (y < margin) y = (anchorBottomY + gap).roundToInt() // 上方不够 → 翻到选区下方
    y = y.coerceIn(margin, (windowH - contentH - margin).coerceAtLeast(margin))
    return IntOffset(x, y)
}
