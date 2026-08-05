package top.yukonga.scripta.editor.completion

import androidx.compose.runtime.Immutable
import top.yukonga.scripta.editor.text.TextPosition
import top.yukonga.scripta.editor.text.TextRange

/** 补全项类型，仅用于候选列表展示，不影响插入行为。 */
enum class CompletionItemKind {
    Property,
    Value,
    Keyword,
}

/**
 * 一条代码补全候选。
 *
 * [label] 是列表中显示的名称，[insertText] 是确认后实际写入文档的文本。
 * [detail] 适合放类型或默认值等短信息，[documentation] 适合放字段说明。
 */
@Immutable
data class CompletionItem(
    val label: String,
    val insertText: String = label,
    val detail: String? = null,
    val documentation: String? = null,
    val kind: CompletionItemKind = CompletionItemKind.Value,
)

/** 编辑器发给补全提供方的不可变文档快照。行列均为 0 基，列使用 UTF-16 下标。 */
@Immutable
data class CompletionRequest(
    val text: String,
    val caret: TextPosition,
    /** true 表示宿主或用户显式请求补全，提供方可放宽自动触发条件。 */
    val explicit: Boolean,
)

/**
 * 一次补全结果。[replaceRange] 是确认候选时被替换的文档范围；候选为空等同于无结果。
 */
@Immutable
data class CompletionResult(
    val replaceRange: TextRange,
    val items: List<CompletionItem>,
)

/**
 * 语言或宿主实现的补全提供方。调用发生在 UI 协程中；耗时解析应自行切换调度器。
 * 新请求会取消上一次尚未完成的调用。
 */
fun interface CompletionProvider {
    suspend fun complete(request: CompletionRequest): CompletionResult?
}
