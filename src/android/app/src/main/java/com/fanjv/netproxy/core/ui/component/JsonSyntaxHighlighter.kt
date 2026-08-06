package com.fanjv.netproxy.core.ui.component

import top.yukonga.scripta.editor.highlight.HighlightSpan
import top.yukonga.scripta.editor.highlight.LineHighlight
import top.yukonga.scripta.editor.highlight.LineState
import top.yukonga.scripta.editor.highlight.SyntaxHighlighter
import top.yukonga.scripta.editor.highlight.TokenType

/** 为 JSON 配置提供逐行增量语法高亮。 */
class JsonSyntaxHighlighter : SyntaxHighlighter {
    override fun highlightLine(text: String, entryState: LineState?): LineHighlight {
        val spans = ArrayList<HighlightSpan>()
        var index = 0

        while (index < text.length) {
            when (val char = text[index]) {
                '"' -> {
                    val end = stringEnd(text, index)
                    val type = if (isObjectKey(text, end)) TokenType.Key else TokenType.String
                    spans.add(HighlightSpan(index, end, type))
                    index = end
                }

                '{', '}', '[', ']', ':', ',' -> {
                    spans.add(HighlightSpan(index, index + 1, TokenType.Punctuation))
                    index++
                }

                '-', in '0'..'9' -> {
                    val end = numberEnd(text, index)
                    spans.add(HighlightSpan(index, end, TokenType.Number))
                    index = end
                }

                else -> {
                    val token = when {
                        text.hasTokenAt(index, "true") -> "true" to TokenType.Boolean
                        text.hasTokenAt(index, "false") -> "false" to TokenType.Boolean
                        text.hasTokenAt(index, "null") -> "null" to TokenType.Null
                        else -> null
                    }
                    if (token == null) {
                        index++
                    } else {
                        val end = index + token.first.length
                        spans.add(HighlightSpan(index, end, token.second))
                        index = end
                    }
                }
            }
        }

        return LineHighlight(spans, null)
    }

    private fun stringEnd(text: String, start: Int): Int {
        var index = start + 1
        while (index < text.length) {
            when (text[index]) {
                '\\' -> index = (index + 2).coerceAtMost(text.length)
                '"' -> return index + 1
                else -> index++
            }
        }
        return text.length
    }

    private fun isObjectKey(text: String, stringEnd: Int): Boolean {
        var index = stringEnd
        while (index < text.length && text[index].isWhitespace()) index++
        return index < text.length && text[index] == ':'
    }

    private fun numberEnd(text: String, start: Int): Int {
        var index = start + 1
        while (index < text.length && text[index] in NUMBER_CHARACTERS) index++
        return index
    }

    private fun String.hasTokenAt(start: Int, token: String): Boolean {
        if (!startsWith(token, start)) return false
        val end = start + token.length
        return end == length || this[end].isWhitespace() || this[end] in TOKEN_DELIMITERS
    }

    private companion object {
        val NUMBER_CHARACTERS =
            setOf('0', '1', '2', '3', '4', '5', '6', '7', '8', '9', '.', 'e', 'E', '+', '-')
        val TOKEN_DELIMITERS = setOf(',', ']', '}')
    }
}
