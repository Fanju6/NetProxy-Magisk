package top.yukonga.scripta.editor.find

/** 一处匹配的扁平 offset 区间 [start, end)。 */
data class MatchRange(val start: Int, val end: Int)

/** 查找请求：查询串 + 大小写 / 整词 / 正则三个开关。 */
data class FindQuery(
    val text: String,
    val caseSensitive: Boolean = false,
    val wholeWord: Boolean = false,
    val regex: Boolean = false,
)

/** 查找结果：匹配列表（升序、互不重叠）+ 无效正则 / 命中上限标记。 */
class FindResult(
    val matches: List<MatchRange>,
    val invalidPattern: Boolean = false,
    val limitHit: Boolean = false,
) {
    companion object {
        val Empty = FindResult(emptyList())
    }
}

/**
 * 全文查找：字面查询经 [Regex.escape] 统一走正则引擎（大小写折叠一致）；零长匹配跳过并前移一位
 * （防 `a*` 这类模式死循环）；整词经边界后置过滤（字面与正则同一套词字符判定，与引擎双击选词一致）。
 * 命中数达 [maxMatches] 即截断并置 [FindResult.limitHit]，钳住超大文档的开销。
 */
fun findAllMatches(text: String, query: FindQuery, maxMatches: Int = 20_000): FindResult {
    if (query.text.isEmpty()) return FindResult.Empty
    val pattern = if (query.regex) query.text else Regex.escape(query.text)
    val options = if (query.caseSensitive) emptySet() else setOf(RegexOption.IGNORE_CASE)
    val re = try {
        Regex(pattern, options)
    } catch (_: Exception) {
        return FindResult(emptyList(), invalidPattern = true)
    }
    val out = ArrayList<MatchRange>()
    var limitHit = false
    var i = 0
    while (i <= text.length) {
        val m = re.find(text, i) ?: break
        val s = m.range.first
        val e = s + m.value.length
        if (e == s) { // 零长匹配：跳过、前移一位再找
            i = s + 1
            continue
        }
        if (query.wholeWord && !isWholeWordAt(text, s, e)) {
            i = s + 1
            continue
        }
        out.add(MatchRange(s, e))
        if (out.size >= maxMatches) {
            limitHit = re.find(text, e) != null
            break
        }
        i = e
    }
    return FindResult(out, limitHit = limitHit)
}

/**
 * 环绕导航：向前找第一个 start ≥ [fromOffset] 的匹配（没有则回绕到第 0 个）；
 * 向后找最后一个 start < [fromOffset] 的匹配（没有则回绕到最后一个）。空列表返回 null。
 */
fun nextMatchIndex(matches: List<MatchRange>, fromOffset: Int, forward: Boolean): Int? {
    if (matches.isEmpty()) return null
    return if (forward) {
        val i = matches.indexOfFirst { it.start >= fromOffset }
        if (i >= 0) i else 0
    } else {
        val i = matches.indexOfLast { it.start < fromOffset }
        if (i >= 0) i else matches.lastIndex
    }
}

/** 词字符判定与 [top.yukonga.scripta.editor.EditorEngine] 的双击选词一致：字母数字 / _ / $。 */
private fun isWordChar(c: Char): Boolean = c.isLetterOrDigit() || c == '_' || c == '$'

private fun isWholeWordAt(text: String, s: Int, e: Int): Boolean =
    (s == 0 || !isWordChar(text[s - 1])) && (e >= text.length || !isWordChar(text[e]))
