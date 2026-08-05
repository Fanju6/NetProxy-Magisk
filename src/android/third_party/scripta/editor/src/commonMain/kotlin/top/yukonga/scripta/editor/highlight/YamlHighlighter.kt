package top.yukonga.scripta.editor.highlight

/**
 * YAML 语法高亮插件（行级增量）。跨行结构只有块标量（`|` / `>`）：进入后把「缩进大于指示符所在行」的
 * 各行整行标为字符串，缩进回落即退出（经 [LineState] 在行间传递，空行保持状态）。行内为一遍游标扫描：
 * 注释 / 文档标记 / 指令整行形态优先；随后序列破折号、键（普通 / 引号 / `<<` 合并键）、值
 * （引号串、锚点 / 引用、标签、流式集合、直接量分类为数字 / 布尔 / 空 / 时间戳 / 字符串）。
 * 直接量在块上下文一直吃到「空格+#」或行尾——其中的 `:`（如时间戳）不再是键分隔。
 */
class YamlHighlighter : SyntaxHighlighter {

    override val lineCommentPrefix: String = "#"

    private data class BlockScalarState(val parentIndent: Int) : LineState

    override fun highlightLine(text: String, entryState: LineState?): LineHighlight {
        if (entryState is BlockScalarState) {
            val indent = firstNonSpace(text)
            if (indent < 0) return LineHighlight(emptyList(), entryState) // 空行保持块标量态
            if (indent > entryState.parentIndent) {
                return LineHighlight(listOf(HighlightSpan(indent, text.length, TokenType.String)), entryState)
            }
            // 缩进回落：块结束，按普通行继续扫描。
        }
        return Scanner(text).scanLine()
    }

    private fun firstNonSpace(t: String): Int {
        var i = 0
        while (i < t.length && t[i] == ' ') i++
        return if (i >= t.length) -1 else i
    }

    private class Scanner(private val text: String) {
        private val spans = ArrayList<HighlightSpan>()
        private var exit: LineState? = null
        private var i = 0
        private var flowDepth = 0

        fun scanLine(): LineHighlight {
            skipSpaces()
            val indent = i
            if (i >= text.length) return done()
            when {
                text[i] == '#' -> {
                    comment(); return done()
                }

                indent == 0 && (text.startsWith("---") || text.startsWith("...")) -> {
                    spans.add(HighlightSpan(0, 3, TokenType.Keyword))
                    i = 3
                    skipSpaces()
                    if (i < text.length) scanValue(indent) // 如 "--- !!map"
                    return done()
                }

                indent == 0 && text[i] == '%' -> {
                    spans.add(HighlightSpan(0, text.length, TokenType.Directive))
                    return done()
                }
            }
            // 序列指示符（可链式嵌套 "- - a"）。
            while (i < text.length && text[i] == '-' && (i + 1 >= text.length || text[i + 1] == ' ')) {
                spans.add(HighlightSpan(i, i + 1, TokenType.Punctuation))
                i += 2
                skipSpaces()
            }
            if (i >= text.length) return done()
            if (text[i] == '#') {
                comment(); return done()
            }
            if (tryParseKey()) skipSpaces()
            scanValue(indent)
            return done()
        }

        // --- 键 ------------------------------------------------------------------------------

        /** 尝试解析「键 + 冒号」；成功则消费并返回 true，失败不动游标。 */
        private fun tryParseKey(): Boolean {
            if (text[i] == '"' || text[i] == '\'') {
                val qe = stringEnd(i)
                var j = qe
                while (j < text.length && text[j] == ' ') j++
                if (j < text.length && text[j] == ':' && (j + 1 >= text.length || text[j + 1] == ' ')) {
                    spans.add(HighlightSpan(i, qe, TokenType.Key))
                    spans.add(HighlightSpan(j, j + 1, TokenType.Punctuation))
                    i = j + 1
                    return true
                }
                return false
            }
            var j = i
            while (j < text.length) {
                val c = text[j]
                if (c == ':' && (j + 1 >= text.length || text[j + 1] == ' ')) {
                    var ke = j
                    while (ke > i && text[ke - 1] == ' ') ke--
                    if (ke == i) return false
                    val kind = if (text.substring(i, ke) == "<<") TokenType.Keyword else TokenType.Key
                    spans.add(HighlightSpan(i, ke, kind))
                    spans.add(HighlightSpan(j, j + 1, TokenType.Punctuation))
                    i = j + 1
                    return true
                }
                if (c == '#' && j > i && text[j - 1] == ' ') return false // 注释前无冒号：非键
                if (c == '[' || c == '{' || c == ']' || c == '}' || c == ',') return false
                j++
            }
            return false
        }

        // --- 值 ------------------------------------------------------------------------------

        private fun scanValue(indent: Int) {
            while (true) {
                skipSpaces()
                if (i >= text.length) return
                when (val c = text[i]) {
                    '#' -> {
                        comment(); return
                    }

                    '"', '\'' -> {
                        val e = stringEnd(i)
                        spans.add(HighlightSpan(i, e, TokenType.String))
                        i = e
                    }

                    '&', '*' -> {
                        val e = tokenEnd(i + 1)
                        spans.add(HighlightSpan(i, e, TokenType.Anchor))
                        i = e
                    }

                    '!' -> {
                        val e = tokenEnd(i + 1)
                        spans.add(HighlightSpan(i, e, TokenType.Tag))
                        i = e
                    }

                    '|', '>' -> {
                        val e = blockIndicatorEnd()
                        if (e > 0 && flowDepth == 0) {
                            spans.add(HighlightSpan(i, e, TokenType.Punctuation))
                            exit = BlockScalarState(indent)
                            i = e
                        } else {
                            scanPlain()
                        }
                    }

                    '[', '{' -> {
                        spans.add(HighlightSpan(i, i + 1, TokenType.Punctuation))
                        flowDepth++
                        i++
                    }

                    ']', '}' -> {
                        spans.add(HighlightSpan(i, i + 1, TokenType.Punctuation))
                        if (flowDepth > 0) flowDepth--
                        i++
                    }

                    ',' -> {
                        spans.add(HighlightSpan(i, i + 1, TokenType.Punctuation))
                        i++
                    }

                    ':' -> {
                        // 流内键值分隔（块上下文的冒号已并入直接量或键解析）。
                        spans.add(HighlightSpan(i, i + 1, TokenType.Punctuation))
                        i++
                    }

                    else -> scanPlain()
                }
            }
        }

        /** 直接量：块上下文吃到「空格+#」/ 行尾；流上下文吃到分隔符。流内后随冒号的 token 是键。 */
        private fun scanPlain() {
            if (flowDepth == 0) {
                var end = text.length
                var k = i
                while (k < text.length - 1) {
                    if (text[k] == ' ' && text[k + 1] == '#') {
                        end = k
                        break
                    }
                    k++
                }
                var e = end
                while (e > i && text[e - 1] == ' ') e--
                if (e > i) classifyAndAdd(i, e)
                i = end
            } else {
                // token 到 , ] } 或「键冒号」为止。冒号仅在后随空格 / 流分隔 / 行尾时才是键值分隔
                // ——无引号 URL（https://…）或带端口地址（host:9090）里的冒号属于标量本身，不切断。
                var j = i
                while (j < text.length) {
                    val ch = text[j]
                    if (ch == ',' || ch == ']' || ch == '}') break
                    if (ch == ':' && isFlowKeyColon(j)) break
                    j++
                }
                var e = j
                while (e > i && text[e - 1] == ' ') e--
                if (e > i) {
                    if (j < text.length && text[j] == ':') {
                        // 停在键冒号上：本 token 是流内键；`<<` 合并键与块上下文同色。
                        val kind = if (text.substring(i, e) == "<<") TokenType.Keyword else TokenType.Key
                        spans.add(HighlightSpan(i, e, kind))
                    } else {
                        classifyAndAdd(i, e)
                    }
                }
                i = j
            }
        }

        /** [pos] 处的冒号是否为流内「键值分隔」：后随空格 / 流分隔符 / 行尾（YAML 流语法的键冒号形态）。 */
        private fun isFlowKeyColon(pos: Int): Boolean {
            val n = pos + 1
            return n >= text.length || text[n] == ' ' || text[n] == ',' || text[n] == ']' || text[n] == '}'
        }

        private fun classifyAndAdd(s: Int, e: Int) {
            val token = text.substring(s, e)
            val type = when {
                token.lowercase() in BOOLS -> TokenType.Boolean
                token == "~" || token.lowercase() == "null" -> TokenType.Null
                NUMBER.matches(token) || DATE.matches(token) -> TokenType.Number
                else -> TokenType.String
            }
            spans.add(HighlightSpan(s, e, type))
        }

        // --- 词法辅助 --------------------------------------------------------------------------

        private fun skipSpaces() {
            while (i < text.length && text[i] == ' ') i++
        }

        private fun comment() {
            spans.add(HighlightSpan(i, text.length, TokenType.Comment))
            i = text.length
        }

        /** 引号串自 [from]（指向引号）到收尾引号之后；未闭合到行尾。双引号 `\` 转义、单引号 `''` 转义。 */
        private fun stringEnd(from: Int): Int {
            val quote = text[from]
            var j = from + 1
            while (j < text.length) {
                val c = text[j]
                if (quote == '"' && c == '\\') {
                    j += 2
                    continue
                }
                if (c == quote) {
                    if (quote == '\'' && j + 1 < text.length && text[j + 1] == '\'') {
                        j += 2
                        continue
                    }
                    return j + 1
                }
                j++
            }
            return text.length
        }

        /** 锚点 / 标签名结束：到空格或流分隔符。 */
        private fun tokenEnd(from: Int): Int {
            var j = from
            while (j < text.length && text[j] != ' ' && text[j] !in ",[]{}") j++
            return j
        }

        /** 块标量指示符（`|`/`>` + 可选 chomp/缩进符）后仅剩空白或注释才成立；返回指示符结束下标，否则 -1。 */
        private fun blockIndicatorEnd(): Int {
            var j = i + 1
            while (j < text.length && (text[j] == '+' || text[j] == '-' || text[j].isDigit())) j++
            var k = j
            while (k < text.length && text[k] == ' ') k++
            return if (k >= text.length || text[k] == '#') j else -1
        }

        private fun done() = LineHighlight(spans, exit)

        companion object {
            private val BOOLS = setOf("true", "false", "yes", "no", "on", "off")
            private val NUMBER = Regex("^[-+]?(0x[0-9a-fA-F]+|0o[0-7]+|(\\d+(\\.\\d*)?|\\.\\d+)([eE][-+]?\\d+)?)$")
            private val DATE = Regex("^\\d{4}-\\d{2}-\\d{2}([Tt ].*)?$")
        }
    }
}
