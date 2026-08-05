@file:OptIn(ExperimentalComposeUiApi::class)

package top.yukonga.scripta.editor.input

import androidx.compose.ui.ExperimentalComposeUiApi
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusEventModifierNode
import androidx.compose.ui.focus.FocusState
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.layout.positionInRoot
import androidx.compose.ui.node.ModifierNodeElement
import androidx.compose.ui.node.requireLayoutCoordinates
import androidx.compose.ui.platform.InspectorInfo
import androidx.compose.ui.platform.PlatformTextInputMethodRequest
import androidx.compose.ui.platform.PlatformTextInputModifierNode
import androidx.compose.ui.platform.establishTextInputSession
import androidx.compose.ui.text.TextLayoutResult
import androidx.compose.ui.text.TextRange
import androidx.compose.ui.text.input.EditCommand
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.ImeOptions
import androidx.compose.ui.text.input.TextEditingScope
import androidx.compose.ui.text.input.TextEditorState
import androidx.compose.ui.text.input.TextFieldValue
import kotlinx.coroutines.Job
import kotlinx.coroutines.launch
import top.yukonga.scripta.editor.EditorEngine
import top.yukonga.scripta.editor.text.TextPosition
import top.yukonga.scripta.editor.text.TextRange as EditorTextRange

/**
 * 桌面（JVM/skiko）自管 IME：非 BasicTextField 的自绘编辑器，经公开的
 * [establishTextInputSession] / [PlatformTextInputMethodRequest] 接入 skiko 输入，
 * 直接喂 Android 路径同一个 [EditorEngine]，拿到字符输入 + CJK 预编辑（拼音候选/下划线/提交）。
 *
 * 桌面的 request 是「拉取 state + 推送 editText」模型（与 Android 的 InputConnection 工厂完全不同）：
 * Compose 的 DesktopTextInputService2 把 AWT InputMethodEvent 翻译成 commitText/setComposingText/
 * finishComposingText，回打到本 request 的 [ScriptaImeRequest.editText]；并跑 snapshotFlow 观测
 * state.selection / state.composition，在光标移动/外部编辑时自动结束预编辑——故这两个 getter 必须读
 * 引擎的快照 state（selectionOffsets/composingOffsets 分别读 engine.selection、engine.composing）。
 */
actual fun Modifier.editorTextInput(engine: EditorEngine, enabled: Boolean, caretRectInEditor: () -> Rect?): Modifier =
    if (enabled) this then EditorTextInputElement(engine, caretRectInEditor) else this

private data class EditorTextInputElement(
    val engine: EditorEngine,
    val caretRectInEditor: () -> Rect?,
) : ModifierNodeElement<EditorTextInputNode>() {
    override fun create(): EditorTextInputNode = EditorTextInputNode(engine, caretRectInEditor)
    override fun update(node: EditorTextInputNode) {
        node.engine = engine
        node.caretRectInEditor = caretRectInEditor
    }

    override fun InspectorInfo.inspectableProperties() {
        name = "editorTextInput"
    }
}

private class EditorTextInputNode(
    var engine: EditorEngine,
    var caretRectInEditor: () -> Rect?,
) : Modifier.Node(), FocusEventModifierNode, PlatformTextInputModifierNode {

    private var focusedJob: Job? = null
    private var focused = false

    override fun onFocusEvent(focusState: FocusState) {
        if (focusState.isFocused == focused) return
        focused = focusState.isFocused
        focusedJob?.cancel()
        focusedJob = if (focused) coroutineScope.launch { runSession() } else null
    }

    private suspend fun runSession() {
        establishTextInputSession {
            startInputMethod(ScriptaImeRequest(engine, ::focusedRectInRoot))
        }
    }

    /**
     * IME 候选窗定位矩形（root 坐标）：把编辑器传入的「光标在内容本地坐标系的矩形」按本节点在 root 中的
     * 左上角平移即得——本节点即挂在编辑器内容 Box 上，故其 positionInRoot 正是该本地坐标系的原点。
     * 拿不到光标矩形（首帧布局前 / 无有效光标）时退回本节点自身矩形，候选窗不贴光标但仍不崩。
     * requireLayoutCoordinates() 在节点/坐标未附着时抛 IllegalStateException（首帧、拆窗），这里吞成 null。
     */
    private fun focusedRectInRoot(): Rect? {
        if (!isAttached) return null
        val coords = try {
            requireLayoutCoordinates()
        } catch (_: IllegalStateException) {
            return null
        }
        if (!coords.isAttached) return null
        val origin = coords.positionInRoot()
        val caret = caretRectInEditor()
        return if (caret != null) caret.translate(origin) else Rect(origin, Size(1f, coords.size.height.toFloat()))
    }

    override fun onDetach() {
        focusedJob?.cancel()
        focusedJob = null
        focused = false
    }
}

/**
 * 桌面的 [PlatformTextInputMethodRequest] actual（skikoMain 的富接口，非 Android 的
 * createInputConnection 单方法）：Compose 从 [state] 拉取文本/选区/预编辑区间与几何，向 [editText]
 * 推送提交/预编辑。全部成员在 1.11.1 被 @ExperimentalComposeUiApi 标注（见文件顶 @file:OptIn）。
 */
private class ScriptaImeRequest(
    private val engine: EditorEngine,
    private val cursorRectInRoot: () -> Rect?,
) : PlatformTextInputMethodRequest {

    /** 引擎活文本上的 CharSequence + 选区/预编辑区间。selection/composition 读快照 state（见类头注释）。 */
    override val state: TextEditorState = object : TextEditorState {
        override val length: Int get() = engine.buffer.totalLength()

        override fun get(index: Int): Char {
            // CharSequence 契约：合法下标 0..length-1。Compose 的 IME 查询遵守边界，此守卫仅防御越界消费者——
            // 否则 index==length 时两个 positionAt 都夹到末尾、textInRange 返回空串、[0] 抛不直观的异常。
            if (index < 0 || index >= length) throw IndexOutOfBoundsException("index=$index, length=$length")
            return engine.buffer.textInRange(
                EditorTextRange(engine.buffer.positionAt(index), engine.buffer.positionAt(index + 1))
            )[0]
        }

        override fun subSequence(startIndex: Int, endIndex: Int): CharSequence =
            engine.buffer.textInRange(
                EditorTextRange(engine.buffer.positionAt(startIndex), engine.buffer.positionAt(endIndex))
            )

        override fun toString(): String = engine.getText()

        override val selection: TextRange
            get() {
                val (s, e) = engine.selectionOffsets()
                return TextRange(s, e)
            }

        override val composition: TextRange?
            get() {
                val (s, e) = engine.composingOffsets()
                return if (s < 0) null else TextRange(s, e)
            }
    }

    override val value: () -> TextFieldValue = {
        TextFieldValue(
            text = engine.getText(),
            selection = state.selection,
            composition = state.composition,
        )
    }

    override val imeOptions: ImeOptions = ImeOptions.Default

    override val onEditCommand: (List<EditCommand>) -> Unit = { /* 桌面预编辑路径不走 onEditCommand，走 editText */ }

    override val onImeAction: ((ImeAction) -> Unit)? = null

    override val textLayoutResult: () -> TextLayoutResult? = { null }

    override val focusedRectInRoot: () -> Rect? = cursorRectInRoot
    override val textFieldRectInRoot: () -> Rect? = cursorRectInRoot
    override val textClippingRectInRoot: () -> Rect? = cursorRectInRoot
    override val unclippedTextOffsetInRoot: () -> Offset? = { cursorRectInRoot()?.topLeft }

    /**
     * Compose 把一次输入法事件里的多次编辑缓冲后，一次性回打本 block。此处在 AWT EDT（桌面即合成线程）
     * 上直接改引擎——引擎各编辑原语自带快照写入，无需额外事务包裹。CharSequence 参数 toString() 后转发。
     */
    override val editText: (block: TextEditingScope.() -> Unit) -> Unit = { block ->
        block(object : TextEditingScope {
            override fun deleteSurroundingTextInCodePoints(lengthBeforeCursor: Int, lengthAfterCursor: Int) =
                engine.deleteSurroundingTextInCodePoints(lengthBeforeCursor, lengthAfterCursor)

            override fun commitText(text: CharSequence, newCursorPosition: Int) =
                engine.commitText(text.toString(), newCursorPosition)

            override fun setComposingText(text: CharSequence, newCursorPosition: Int) =
                engine.setComposingText(text.toString(), newCursorPosition)

            override fun finishComposingText() =
                engine.finishComposing()
        })
    }
}
