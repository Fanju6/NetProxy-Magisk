package top.yukonga.scripta.editor.input

import android.text.InputType
import android.text.TextUtils
import android.view.KeyEvent
import android.view.View
import android.view.inputmethod.BaseInputConnection
import android.view.inputmethod.EditorInfo
import android.view.inputmethod.ExtractedText
import android.view.inputmethod.ExtractedTextRequest
import android.view.inputmethod.InputMethodManager
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusEventModifierNode
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.focus.FocusState
import androidx.compose.ui.node.ModifierNodeElement
import androidx.compose.ui.platform.InspectorInfo
import androidx.compose.ui.platform.PlatformTextInputMethodRequest
import androidx.compose.ui.platform.PlatformTextInputModifierNode
import androidx.compose.ui.platform.establishTextInputSession
import kotlinx.coroutines.Job
import kotlinx.coroutines.launch
import top.yukonga.scripta.editor.EditorEngine

/** getExtractedText 返回的光标上下文窗口半径（字符）：够 IME 联想/上下文用，又不随文档大小 O(n)。 */
private const val EXTRACT_WINDOW = 2048

/** 自管 IME 的 InputConnection：BaseInputConnection 子类，全部文本操作转发到 EditorEngine。 */
internal class EditorInputConnection(
    view: View,
    private val engine: EditorEngine,
) : BaseInputConnection(view, /* fullEditor = */ false) {

    override fun beginBatchEdit(): Boolean {
        engine.beginBatch(); return true
    }

    override fun endBatchEdit(): Boolean = engine.endBatch()

    override fun setComposingText(text: CharSequence, newCursorPosition: Int): Boolean {
        engine.setComposingText(text.toString(), newCursorPosition); return true
    }

    override fun setComposingRegion(start: Int, end: Int): Boolean {
        engine.setComposingRegion(start, end); return true
    }

    override fun finishComposingText(): Boolean {
        engine.finishComposing(); return true
    }

    override fun commitText(text: CharSequence, newCursorPosition: Int): Boolean {
        // 有的输入法把回车键发成 commitText("\n")：无预编辑时等同回车键，走缩进继承；
        // 预编辑中的 "\n" 仍按提交语义处理（结束会话入文），那是会话文本、不是回车键。
        if (engine.composing == null && text.length == 1 && text[0] == '\n') {
            engine.insertNewlineAutoIndent(); return true
        }
        // 软键盘的普通单字符提交 = 键入：走键入原语（自动配对/跳过闭合）。仅限无预编辑且
        // newCursorPosition==1（标准打字形态）；预编辑提交/整词候选/含格式的 commit 照走原语义。
        if (engine.composing == null && text.length == 1 && newCursorPosition == 1) {
            engine.typeCharacter(text.toString()); return true
        }
        engine.commitText(text.toString(), newCursorPosition); return true
    }

    override fun deleteSurroundingText(beforeLength: Int, afterLength: Int): Boolean {
        engine.deleteSurroundingText(beforeLength, afterLength); return true
    }

    override fun deleteSurroundingTextInCodePoints(beforeLength: Int, afterLength: Int): Boolean {
        engine.deleteSurroundingTextInCodePoints(beforeLength, afterLength); return true
    }

    override fun getTextBeforeCursor(n: Int, flags: Int): CharSequence = engine.textBeforeCursor(n)
    override fun getTextAfterCursor(n: Int, flags: Int): CharSequence = engine.textAfterCursor(n)
    override fun getSelectedText(flags: Int): CharSequence? = engine.selectedText()

    override fun getCursorCapsMode(reqModes: Int): Int {
        // 有界窗口即可（避免大文件 O(n)）。
        val before = engine.textBeforeCursor(256).toString()
        return TextUtils.getCapsMode(before, before.length, reqModes)
    }

    override fun getExtractedText(request: ExtractedTextRequest?, flags: Int): ExtractedText {
        // 只返回光标周围的有界窗口，不再 getText() 拼全篇——大文件下那正是 OOM 来源。已设 IME_FLAG_NO_EXTRACT_UI，
        // 主流输入法几乎不走提取式全文编辑；偏移相对窗口换算，selection 仍准确。partialStartOffset=-1 表示 text
        // 即 [startOffset, startOffset+len) 的完整内容（非增量）。
        val before = engine.textBeforeCursor(EXTRACT_WINDOW).toString()
        val selected = engine.selectedText() ?: ""
        val after = engine.textAfterCursor(EXTRACT_WINDOW).toString()
        val (s, _) = engine.selectionOffsets()
        return ExtractedText().apply {
            text = before + selected + after
            startOffset = s - before.length
            partialStartOffset = -1
            partialEndOffset = -1
            selectionStart = before.length
            selectionEnd = before.length + selected.length
        }
    }

    override fun sendKeyEvent(event: KeyEvent): Boolean {
        if (event.action == KeyEvent.ACTION_DOWN) {
            when (event.keyCode) {
                KeyEvent.KEYCODE_DEL -> {
                    engine.backspace(); return true
                }

                KeyEvent.KEYCODE_FORWARD_DEL -> {
                    engine.deleteForward(); return true
                }

                KeyEvent.KEYCODE_ENTER -> {
                    engine.insertNewlineAutoIndent(); return true
                }

                else -> {
                    val ch = event.unicodeChar
                    if (ch != 0) {
                        engine.typeCharacter(String(Character.toChars(ch))); return true // 键入原语：带自动配对
                    }
                }
            }
        }
        return super.sendKeyEvent(event)
    }
}

// caretRectInEditor 在 Android 忽略：候选栏/软键盘位置由系统输入法自行决定。
actual fun Modifier.editorTextInput(engine: EditorEngine, enabled: Boolean, caretRectInEditor: () -> Rect?): Modifier =
    if (enabled) this then EditorTextInputElement(engine) else this

private data class EditorTextInputElement(val engine: EditorEngine) :
    ModifierNodeElement<EditorTextInputNode>() {
    override fun create(): EditorTextInputNode = EditorTextInputNode(engine)
    override fun update(node: EditorTextInputNode) {
        node.engine = engine
    }

    override fun InspectorInfo.inspectableProperties() {
        name = "editorTextInput"
    }
}

private class EditorTextInputNode(var engine: EditorEngine) :
    Modifier.Node(), FocusEventModifierNode, PlatformTextInputModifierNode {

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
            val v = view
            val imm = v.context.getSystemService(InputMethodManager::class.java)
            engine.imeListener = object : EditorEngine.ImeListener {
                override fun onSelectionChanged(selStart: Int, selEnd: Int, composingStart: Int, composingEnd: Int) {
                    imm?.updateSelection(v, selStart, selEnd, composingStart, composingEnd)
                }
            }
            engine.requestShowKeyboard = { imm?.showSoftInput(v, 0) }
            // 换文档时重启输入连接：预编辑进行中被 setText 单方清掉时，仅靠 updateSelection(composing=-1)
            // 不足以让所有输入法丢弃旧组合状态，残留候选可能提交进新文档的 (0,0)。
            engine.onDocumentReplaced = { imm?.restartInput(v) }
            val request = PlatformTextInputMethodRequest { outAttrs ->
                // NO_SUGGESTIONS：代码/YAML 不要候选条与拼写波浪线，避免输入法把 fooBar、i++ 等联想改坏。
                outAttrs.inputType =
                    InputType.TYPE_CLASS_TEXT or InputType.TYPE_TEXT_FLAG_MULTI_LINE or InputType.TYPE_TEXT_FLAG_NO_SUGGESTIONS
                // NO_PERSONALIZED_LEARNING（API 26+，旧版忽略）：别把代码喂进输入法个人词库。
                outAttrs.imeOptions =
                    EditorInfo.IME_ACTION_NONE or EditorInfo.IME_FLAG_NO_FULLSCREEN or
                            EditorInfo.IME_FLAG_NO_EXTRACT_UI or EditorInfo.IME_FLAG_NO_PERSONALIZED_LEARNING
                val (s, e) = engine.selectionOffsets()
                outAttrs.initialSelStart = s
                outAttrs.initialSelEnd = e
                EditorInputConnection(v, engine)
            }
            try {
                startInputMethod(request)
            } finally {
                engine.imeListener = null
                engine.requestShowKeyboard = null
                engine.onDocumentReplaced = null
            }
        }
    }

    override fun onDetach() {
        focusedJob?.cancel()
        focusedJob = null
        focused = false
        engine.imeListener = null
        engine.requestShowKeyboard = null
        engine.onDocumentReplaced = null
    }
}
