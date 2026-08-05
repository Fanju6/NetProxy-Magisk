package top.yukonga.scripta.editor.highlight

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue

class YamlHighlighterTest {

    private val h = YamlHighlighter()

    private fun spans(text: String, entry: LineState? = null): List<HighlightSpan> =
        h.highlightLine(text, entry).spans

    private fun typeAt(text: String, col: Int, entry: LineState? = null): TokenType? =
        spans(text, entry).firstOrNull { col >= it.start && col < it.end }?.type

    // --- 整行形态 ----------------------------------------------------------------------------

    @Test
    fun commentLineIsComment() {
        assertEquals(listOf(HighlightSpan(0, 7, TokenType.Comment)), spans("# hello"))
        assertEquals(listOf(HighlightSpan(2, 8, TokenType.Comment)), spans("  # note"))
    }

    @Test
    fun documentMarkersAreKeyword() {
        assertEquals(listOf(HighlightSpan(0, 3, TokenType.Keyword)), spans("---"))
        assertEquals(listOf(HighlightSpan(0, 3, TokenType.Keyword)), spans("..."))
    }

    @Test
    fun directiveLineIsDirective() {
        assertEquals(listOf(HighlightSpan(0, 9, TokenType.Directive)), spans("%YAML 1.2"))
    }

    @Test
    fun emptyLineHasNoSpans() {
        assertTrue(spans("").isEmpty())
        assertTrue(spans("    ").isEmpty())
    }

    // --- 键与标量值 --------------------------------------------------------------------------

    @Test
    fun keyColonPlainValue() {
        // "name: scripta" → 键 [0,4) / 冒号 [4,5) / 直接量字符串值 [6,13)
        assertEquals(
            listOf(
                HighlightSpan(0, 4, TokenType.Key),
                HighlightSpan(4, 5, TokenType.Punctuation),
                HighlightSpan(6, 13, TokenType.String),
            ),
            spans("name: scripta"),
        )
    }

    @Test
    fun numericValues() {
        assertEquals(TokenType.Number, typeAt("int: 42", 5))
        assertEquals(TokenType.Number, typeAt("negative: -17", 10))
        assertEquals(TokenType.Number, typeAt("float: 3.14159", 7))
        assertEquals(TokenType.Number, typeAt("scientific: 6.022e23", 12))
        assertEquals(TokenType.Number, typeAt("hex: 0x1F", 5))
        assertEquals(TokenType.Number, typeAt("octal: 0o17", 7))
    }

    @Test
    fun timestampIsNumber() {
        assertEquals(TokenType.Number, typeAt("released: 2026-07-06", 10))
        assertEquals(TokenType.Number, typeAt("ts: 2026-07-06T21:00:00Z", 4))
    }

    @Test
    fun dottedVersionIsPlainString() {
        assertEquals(TokenType.String, typeAt("version: 0.1.0", 9))
    }

    @Test
    fun booleanAndNullValues() {
        assertEquals(TokenType.Boolean, typeAt("stable: true", 8))
        assertEquals(TokenType.Boolean, typeAt("flag: off", 6))
        assertEquals(TokenType.Null, typeAt("maintainer: null", 12))
        assertEquals(TokenType.Null, typeAt("homepage: ~", 10))
    }

    @Test
    fun trailingCommentAfterValue() {
        val s = spans("maintainer: null   # 空值")
        assertEquals(TokenType.Null, s.first { it.start == 12 }.type)
        val comment = s.last()
        assertEquals(TokenType.Comment, comment.type)
        assertEquals(19, comment.start)
    }

    @Test
    fun quotedValueKeepsHashAndColonInside() {
        // 引号内的 # 与 : 不是注释/键分隔
        val text = "quoted: \"含 : 冒号与 # 井号\""
        val s = spans(text)
        assertEquals(TokenType.Key, s[0].type)
        val str = s.last()
        assertEquals(TokenType.String, str.type)
        assertEquals(8, str.start)
        assertEquals(text.length, str.end)
    }

    @Test
    fun singleQuoteEscapeStaysInString() {
        val text = "single: 'a '' b'"
        val str = spans(text).last()
        assertEquals(TokenType.String, str.type)
        assertEquals(text.length, str.end)
    }

    @Test
    fun quotedKeyIsKey() {
        val s = spans("\"a: b\": v")
        assertEquals(TokenType.Key, s[0].type)
        assertEquals(0, s[0].start)
        assertEquals(6, s[0].end)
    }

    // --- 序列 / 流式 -------------------------------------------------------------------------

    @Test
    fun sequenceDashIsPunctuation() {
        val s = spans("  - item")
        assertEquals(HighlightSpan(2, 3, TokenType.Punctuation), s[0])
        assertEquals(TokenType.String, typeAt("  - item", 5))
    }

    @Test
    fun flowListBracketsCommasAndScalars() {
        val text = "bools: [true, off]"
        assertEquals(TokenType.Punctuation, typeAt(text, 7))  // [
        assertEquals(TokenType.Boolean, typeAt(text, 8))      // true
        assertEquals(TokenType.Punctuation, typeAt(text, 12)) // ,
        assertEquals(TokenType.Boolean, typeAt(text, 14))     // off
        assertEquals(TokenType.Punctuation, typeAt(text, 17)) // ]
    }

    @Test
    fun flowMapKeysAndValues() {
        val text = "gestures: {tap: fall, pinch: zoom}"
        assertEquals(TokenType.Key, typeAt(text, 11))      // tap
        assertEquals(TokenType.String, typeAt(text, 16))   // fall
        assertEquals(TokenType.Key, typeAt(text, 22))      // pinch
        assertEquals(TokenType.String, typeAt(text, 29))   // zoom
    }

    @Test
    fun flowCommentAfterCloseBracket() {
        val text = "bools: [1]   # 流式"
        val comment = spans(text).last()
        assertEquals(TokenType.Comment, comment.type)
        assertEquals(13, comment.start)
    }

    // --- 锚点 / 引用 / 合并 / 标签 -----------------------------------------------------------

    @Test
    fun anchorAndAlias() {
        assertEquals(TokenType.Anchor, typeAt("defaults: &defaults", 10))
        assertEquals(TokenType.Anchor, typeAt("  <<: *defaults", 6))
    }

    @Test
    fun mergeKeyIsKeyword() {
        val s = spans("  <<: *defaults")
        assertEquals(HighlightSpan(2, 4, TokenType.Keyword), s[0])
    }

    @Test
    fun tagIsTag() {
        assertEquals(TokenType.Tag, typeAt("v: !!str 123", 3))
    }

    // --- 块标量（跨行状态） ------------------------------------------------------------------

    @Test
    fun blockScalarIndicatorOpensState() {
        val r = h.highlightLine("literal: |", null)
        assertEquals(TokenType.Punctuation, r.spans.last().type)
        assertNotNull(r.exitState)
    }

    @Test
    fun foldedIndicatorWithChompOpensState() {
        assertNotNull(h.highlightLine("folded: >-", null).exitState)
    }

    @Test
    fun blockScalarBodyIsStringAndKeepsState() {
        val open = h.highlightLine("literal: |", null)
        val body = h.highlightLine("  第一行保留换行", open.exitState)
        assertEquals(listOf(HighlightSpan(2, 9, TokenType.String)), body.spans)
        assertEquals(open.exitState, body.exitState)
    }

    @Test
    fun blankLineInsideBlockScalarKeepsState() {
        val open = h.highlightLine("literal: |", null)
        val blank = h.highlightLine("", open.exitState)
        assertTrue(blank.spans.isEmpty())
        assertEquals(open.exitState, blank.exitState)
    }

    @Test
    fun dedentExitsBlockScalar() {
        val open = h.highlightLine("literal: |", null)
        val after = h.highlightLine("next: 1", open.exitState)
        assertNull(after.exitState)
        assertEquals(TokenType.Key, after.spans[0].type)
        assertEquals(TokenType.Number, after.spans.last().type)
    }

    @Test
    fun indentedKeyBlockScalarUsesOwnIndent() {
        val open = h.highlightLine("  inner: |", null)
        val body = h.highlightLine("    text", open.exitState)
        assertEquals(TokenType.String, body.spans.single().type)
        val out = h.highlightLine("  sibling: 2", open.exitState)
        assertNull(out.exitState)
        assertEquals(TokenType.Key, out.spans[0].type)
    }

    // --- 流式集合里的「非键冒号」（无引号 URL / 带端口地址） ---------------------------------

    @Test
    fun flowUnquotedUrlIsOneScalar() {
        // flow 键冒号须后随空格/分隔/行尾；URL 里的 :// 不满足 → 整个 URL 是一个标量，https 不是键。
        val text = "a: {icon: https://x.com/y.png}"
        assertEquals(TokenType.Key, typeAt(text, 4))       // icon
        assertEquals(TokenType.String, typeAt(text, 10))   // h
        assertEquals(TokenType.String, typeAt(text, 15))   // ':' 属于 URL
        assertEquals(TokenType.String, typeAt(text, 28))   // g（.png 尾）
        assertEquals(TokenType.Punctuation, typeAt(text, 29)) // }
    }

    @Test
    fun flowValueWithPortColonIsOneScalar() {
        val text = "x: {addr: 127.0.0.1:9090, a: 1}"
        assertEquals(TokenType.Key, typeAt(text, 4))       // addr
        assertEquals(TokenType.String, typeAt(text, 10))   // 127...
        assertEquals(TokenType.String, typeAt(text, 21))   // 9090 与 IP 同段
        assertEquals(TokenType.Key, typeAt(text, 26))      // a 仍是键
        assertEquals(TokenType.Number, typeAt(text, 29))   // 1
    }

    @Test
    fun flowMergeKeyIsKeyword() {
        val text = "y: {<<: *pr, url: v}"
        assertEquals(TokenType.Keyword, typeAt(text, 4))   // <<
        assertEquals(TokenType.Anchor, typeAt(text, 8))    // *pr
        assertEquals(TokenType.Key, typeAt(text, 13))      // url
    }

    @Test
    fun flowSpacedKeyColonStillKey() {
        val text = "m: {a : 1}"
        assertEquals(TokenType.Key, typeAt(text, 4))
        assertEquals(TokenType.Number, typeAt(text, 8))
    }

    // --- CJK 键值 ----------------------------------------------------------------------------

    @Test
    fun cjkKeyAndValue() {
        val text = "中文: 输入测试"
        val s = spans(text)
        assertEquals(HighlightSpan(0, 2, TokenType.Key), s[0])
        assertEquals(TokenType.String, s.last().type)
    }

    // --- 真实配置样本：着色段不变量（区间合法、升序、互不重叠） ------------------------------

    @Test
    fun realWorldConfigLinesProduceValidSpans() {
        val lines = listOf(
            "# 锚点",
            "pr: &pr {type: select, proxies: [节点选择, 香港自动, 全球直连]}",
            "proxy-providers:",
            "  atri:",
            "    type: http",
            "    interval: 86400",
            "    health-check:",
            "        enable: true",
            "        url: \"https://www.gstatic.com/generate_204\"",
            "global-ua: clash.meta",
            "    ",
            "external-controller: 127.0.0.1:9090",
            "external-ui: ./dashboard",
            "secret: 0020",
            "  force-domain:",
            "    - +.v2ex.com",
            "    - \"Mijia Cloud\"",
            "    - '+.wechat.com'",
            "    - \"*\"",
            "    - time.*.com",
            "    - tls://223.5.5.5",
            "    - https://doh.pub/dns-query",
            "  'rule-set:bank_cn_domain,cn_domain':",
            "    'rule-set:geolocation-!cn': ",
            "  - name: \"🟢 直连\"",
            "  - {name: AI, <<: *pr, proxies: [节点选择, 日本自动], icon: \"https://raw.githubusercontent.com/x/ai.png\"}",
            "  - {name: Emby, <<: *pr, icon: https://raw.githubusercontent.com/x/emby.png}",
            "  - {name: 香港节点, type: select, include-all: true, filter: \"(?i)(香港|hk|hongkong|Hong Kong)\", icon: \"https://x/hk.png\"}",
            "      ports: [80, 8080-8880]",
            "  - RULE-SET,ai!cn_domain,AI",
            "  - MATCH,漏网之鱼",
            "  ip: &ip {type: http, interval: 86400, behavior: ipcidr, format: mrs}",
            "  ai!cn_domain: { <<: *domain, url: \"https://raw.githubusercontent.com/x.mrs\" }",
            "  geolocation-!cn: { <<: *domain, url: \"https://x.mrs\" }",
        )
        var state: LineState? = null
        for (line in lines) {
            val r = h.highlightLine(line, state)
            state = r.exitState
            var prevEnd = 0
            for (s in r.spans) {
                assertTrue(s.end > s.start, "空/反向区间 in \"$line\": ${r.spans}")
                assertTrue(s.start >= prevEnd, "乱序/重叠 in \"$line\": ${r.spans}")
                assertTrue(s.end <= line.length, "越界 in \"$line\": ${r.spans}")
                prevEnd = s.end
            }
        }
        assertNull(state) // 全程无跨行结构挂起
    }

    @Test
    fun realWorldSpotChecks() {
        // 含 ! 的普通键仍是键。
        assertEquals(TokenType.Key, typeAt("  geolocation-!cn: { a: 1 }", 2))
        // 引号键内含冒号/逗号不拆键。
        val quoted = "  'rule-set:bank_cn,cn_domain':"
        val s = spans(quoted)
        assertEquals(TokenType.Key, s[0].type)
        assertEquals(2, s[0].start)
        assertEquals(30, s[0].end)
        // 块上下文带端口的地址整段一个标量。
        val addr = "external-controller: 127.0.0.1:9090"
        assertEquals(TokenType.String, typeAt(addr, 21))
        assertEquals(TokenType.String, typeAt(addr, 34))
    }
}
