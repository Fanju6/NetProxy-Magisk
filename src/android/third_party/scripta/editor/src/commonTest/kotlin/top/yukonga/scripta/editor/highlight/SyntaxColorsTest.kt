package top.yukonga.scripta.editor.highlight

import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.font.FontWeight
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

class SyntaxColorsTest {

    @Test
    fun tokenStyleDefaultsToColorOnly() {
        val s = TokenStyle(Color.Red)
        assertEquals(Color.Red, s.color)
        assertNull(s.fontWeight)
        assertNull(s.fontStyle)
        assertNull(s.textDecoration)
    }

    @Test
    fun everyTokenTypeHasItsOwnSlot() {
        val c = SyntaxColors.Dark
        // 穷举 when 保证漏配是编译错误；这里抽查槽位与类型的对应关系没有串位。
        assertEquals(c.comment, c.styleFor(TokenType.Comment))
        assertEquals(c.operator, c.styleFor(TokenType.Operator))
        assertEquals(c.variable, c.styleFor(TokenType.Variable))
        assertEquals(c.property, c.styleFor(TokenType.Property))
        assertEquals(c.function, c.styleFor(TokenType.Function))
        assertEquals(c.type, c.styleFor(TokenType.Type))
        assertEquals(c.escape, c.styleFor(TokenType.Escape))
        assertEquals(c.regex, c.styleFor(TokenType.Regex))
        assertEquals(c.heading, c.styleFor(TokenType.Heading))
    }

    @Test
    fun newSlotsHaveDefaultsSoElevenSlotConstructionStillCompiles() {
        // 只显式给出既有 11 个槽位也能构造：新槽位取默认值（与 Dark 预设一致），老调用点不破坏。
        val c = SyntaxColors(
            comment = TokenStyle(Color.Gray),
            key = TokenStyle(Color.Blue),
            string = TokenStyle(Color.Red),
            number = TokenStyle(Color.Green),
            boolean = TokenStyle(Color.Cyan),
            nullLike = TokenStyle(Color.Cyan),
            keyword = TokenStyle(Color.Magenta),
            punctuation = TokenStyle(Color.DarkGray),
            anchor = TokenStyle(Color.Yellow),
            tag = TokenStyle(Color.Yellow),
            directive = TokenStyle(Color.Magenta),
        )
        assertEquals(SyntaxColors.Dark.operator, c.operator)
        assertEquals(SyntaxColors.Dark.heading, c.heading)
    }

    @Test
    fun highlightedTextCarriesFullTokenStyle() {
        val colors = SyntaxColors.Dark.copy(
            comment = TokenStyle(Color.Green, fontStyle = FontStyle.Italic),
            heading = TokenStyle(Color.Blue, fontWeight = FontWeight.Bold),
        )
        val spans = listOf(
            HighlightSpan(0, 1, TokenType.Comment),
            HighlightSpan(1, 2, TokenType.Heading),
        )
        val text = highlightedText("ab", spans, colors)
        assertEquals(2, text.spanStyles.size)
        assertEquals(Color.Green, text.spanStyles[0].item.color)
        assertEquals(FontStyle.Italic, text.spanStyles[0].item.fontStyle)
        assertEquals(Color.Blue, text.spanStyles[1].item.color)
        assertEquals(FontWeight.Bold, text.spanStyles[1].item.fontWeight)
    }

    @Test
    fun highlightedTextStillClampsOutOfRangeSpans() {
        val text = highlightedText("ab", listOf(HighlightSpan(1, 99, TokenType.String)), SyntaxColors.Dark)
        assertEquals(1, text.spanStyles.size)
        assertEquals(1, text.spanStyles[0].start)
        assertEquals(2, text.spanStyles[0].end)
    }
}
