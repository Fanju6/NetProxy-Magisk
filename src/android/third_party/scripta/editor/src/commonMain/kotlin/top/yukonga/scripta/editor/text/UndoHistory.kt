package top.yukonga.scripta.editor.text

/** 一次缓冲替换的增量：把 [offset, offset+removed.length) 的 [removed] 换成 [inserted]（扁平 UTF-16 offset）。 */
data class TextEdit(val offset: Int, val removed: String, val inserted: String)

/** 选区快照（anchor/head 扁平 offset）：显式存两端以保留方向，恢复时不丢反向选区。 */
data class SelectionState(val anchor: Int, val head: Int)

/** 编辑种类：决定撤销单元的合并规则（见 [UndoHistory.record]）。 */
enum class EditKind { Typing, DeleteBackward, DeleteForward, Composing, Other }

/** 撤销 / 重做要回放的一步：按序应用 [edits]，再把选区恢复到 [selection]。 */
class UndoStep(val edits: List<TextEdit>, val selection: SelectionState)

/**
 * 撤销历史：零 Compose 依赖的纯模型，只操作扁平 offset 增量与选区快照，便于纯单测；
 * 增量的应用（offset↔位置换算、buffer.replace）由调用方（EditorEngine）完成。
 *
 * 一个撤销单元 = 若干增量 + 编辑前/后选区。record 按 [EditKind] 就地合并进上一单元：
 *  - [EditKind.Typing]：与上一输入单元首尾相接（offset 精确接续）且不含换行 → 追加进同一单元，
 *    一次撤销回退整段连续键入；含换行的输入自成单元且封口（换行是自然的语义断点）。
 *  - [EditKind.DeleteBackward] / [EditKind.DeleteForward]：连续退格 / 前删（offset 反向 / 原地接续）并成一段。
 *  - [EditKind.Composing]：预编辑逐键都是「同起点整区替换」，用 replace-of-replace 净化
 *    （新增量的 removed == 上一净增量的 inserted → 折叠为 原 removed → 新 inserted），
 *    整个会话（到提交/取消）只留一个净单元；净效果为空（取消）时整个单元丢弃。
 *  - [EditKind.Other]：从不合并（粘贴 / 选区替换 / 多字符提交各自成单元）。
 *
 * [breakMerge] 给最近单元封口（光标移动 / 会话提交等语义断点后不再并入）。[beginGroup]/[endGroup]
 * 把多次 record 归入同一单元（如 IME 的 surrounding 双段删除），撤销时逆序回放、重做按原序回放。
 * 新 record 一律清空重做栈；单元数超 [maxUnits] 丢最老，钳住内存。
 */
class UndoHistory(private val maxUnits: Int = 1000) {

    private class HistoryUnit(
        val edits: MutableList<TextEdit>,
        val selBefore: SelectionState,
        var selAfter: SelectionState,
        val kind: EditKind,
        var sealed: Boolean = false,
    )

    private val undoStack = ArrayDeque<HistoryUnit>()
    private val redoStack = ArrayDeque<HistoryUnit>()
    private var grouping = false
    private var groupUnit: HistoryUnit? = null

    val canUndo: Boolean get() = undoStack.isNotEmpty()
    val canRedo: Boolean get() = redoStack.isNotEmpty()

    fun record(edit: TextEdit, selBefore: SelectionState, selAfter: SelectionState, kind: EditKind) {
        if (edit.removed.isEmpty() && edit.inserted.isEmpty()) return // 真·无操作，不进历史
        redoStack.clear()

        if (grouping) {
            val g = groupUnit
            if (g == null) {
                val unit = HistoryUnit(mutableListOf(edit), selBefore, selAfter, EditKind.Other)
                undoStack.addLast(unit)
                groupUnit = unit
                trim()
            } else {
                g.edits.add(edit)
                g.selAfter = selAfter
            }
            return
        }

        val last = undoStack.lastOrNull()
        if (last != null && !last.sealed && last.kind == kind && last.edits.size == 1) {
            val prev = last.edits[0]
            when (kind) {
                EditKind.Typing -> if (
                    edit.removed.isEmpty() && prev.removed.isEmpty() &&
                    !edit.inserted.contains('\n') && !prev.inserted.contains('\n') &&
                    edit.offset == prev.offset + prev.inserted.length
                ) {
                    last.edits[0] = prev.copy(inserted = prev.inserted + edit.inserted)
                    last.selAfter = selAfter
                    return
                }

                EditKind.DeleteBackward -> if (
                    edit.inserted.isEmpty() && prev.inserted.isEmpty() &&
                    edit.offset + edit.removed.length == prev.offset
                ) {
                    last.edits[0] = TextEdit(edit.offset, edit.removed + prev.removed, "")
                    last.selAfter = selAfter
                    return
                }

                EditKind.DeleteForward -> if (
                    edit.inserted.isEmpty() && prev.inserted.isEmpty() &&
                    edit.offset == prev.offset
                ) {
                    last.edits[0] = prev.copy(removed = prev.removed + edit.removed)
                    last.selAfter = selAfter
                    return
                }

                EditKind.Composing -> if (edit.offset == prev.offset && edit.removed == prev.inserted) {
                    val net = TextEdit(prev.offset, prev.removed, edit.inserted)
                    if (net.removed.isEmpty() && net.inserted.isEmpty()) {
                        undoStack.removeLast() // 会话净效果为空（取消预编辑）：整个单元丢弃
                    } else {
                        last.edits[0] = net
                        last.selAfter = selAfter
                    }
                    return
                }

                EditKind.Other -> Unit // 从不合并
            }
        }

        val unit = HistoryUnit(mutableListOf(edit), selBefore, selAfter, kind)
        // 含换行的键入自成单元并封口：换行是语义断点，其后键入不并入。
        if (kind == EditKind.Typing && edit.inserted.contains('\n')) unit.sealed = true
        undoStack.addLast(unit)
        trim()
    }

    /** 给最近单元封口：其后的编辑不再并入（光标移动 / 预编辑提交等语义断点）。分组进行中不拆组。 */
    fun breakMerge() {
        if (!grouping) undoStack.lastOrNull()?.sealed = true
    }

    fun beginGroup() {
        grouping = true
        groupUnit = null
    }

    fun endGroup() {
        grouping = false
        groupUnit?.sealed = true
        groupUnit = null
    }

    fun undo(): UndoStep? {
        val unit = undoStack.removeLastOrNull() ?: return null
        unit.sealed = true // 回到重做栈再回来时不再接受合并
        redoStack.addLast(unit)
        val inverse = unit.edits.asReversed().map { TextEdit(it.offset, it.inserted, it.removed) }
        return UndoStep(inverse, unit.selBefore)
    }

    fun redo(): UndoStep? {
        val unit = redoStack.removeLastOrNull() ?: return null
        undoStack.addLast(unit)
        return UndoStep(unit.edits.toList(), unit.selAfter)
    }

    fun clear() {
        undoStack.clear()
        redoStack.clear()
        groupUnit = null
        grouping = false
    }

    private fun trim() {
        while (undoStack.size > maxUnits) undoStack.removeFirst()
    }
}
