package top.yukonga.scripta.editor.find

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

class FindMatchesTest {

    private fun m(vararg pairs: Pair<Int, Int>) = pairs.map { MatchRange(it.first, it.second) }

    @Test
    fun literalFindsAllOccurrences() {
        val r = findAllMatches("ab ab ab", FindQuery("ab"))
        assertEquals(m(0 to 2, 3 to 5, 6 to 8), r.matches)
        assertFalse(r.invalidPattern)
        assertFalse(r.limitHit)
    }

    @Test
    fun caseInsensitiveByDefault() {
        val r = findAllMatches("Ab aB AB", FindQuery("ab"))
        assertEquals(3, r.matches.size)
    }

    @Test
    fun caseSensitiveRespectsCase() {
        val r = findAllMatches("Ab aB ab", FindQuery("ab", caseSensitive = true))
        assertEquals(m(6 to 8), r.matches)
    }

    @Test
    fun wholeWordFiltersSubstringHits() {
        val r = findAllMatches("cat catalog concat cat", FindQuery("cat", wholeWord = true))
        assertEquals(m(0 to 3, 19 to 22), r.matches)
    }

    @Test
    fun wholeWordTreatsUnderscoreAndDollarAsWordChars() {
        val r = findAllMatches("my_var var \$var var", FindQuery("var", wholeWord = true))
        // my_var 与 $var 里的 var 不是独立词；两个裸 var 是。
        assertEquals(m(7 to 10, 16 to 19), r.matches)
    }

    @Test
    fun regexFindsMatches() {
        val r = findAllMatches("a1 b22 c333", FindQuery("[a-z](\\d+)", regex = true))
        assertEquals(m(0 to 2, 3 to 6, 7 to 11), r.matches)
    }

    @Test
    fun invalidRegexReportsErrorWithoutMatches() {
        val r = findAllMatches("abc", FindQuery("[", regex = true))
        assertTrue(r.invalidPattern)
        assertTrue(r.matches.isEmpty())
    }

    @Test
    fun zeroLengthRegexMatchesAreSkipped() {
        val r = findAllMatches("baab", FindQuery("a*", regex = true))
        assertEquals(m(1 to 3), r.matches) // 只留非空命中，零长不进结果也不死循环
    }

    @Test
    fun emptyQueryHasNoMatches() {
        assertTrue(findAllMatches("abc", FindQuery("")).matches.isEmpty())
    }

    @Test
    fun maxMatchesCapsAndFlags() {
        val r = findAllMatches("aaaaaaaaaa", FindQuery("a"), maxMatches = 4)
        assertEquals(4, r.matches.size)
        assertTrue(r.limitHit)
    }

    @Test
    fun literalQueryMayCrossLines() {
        val r = findAllMatches("x\ny x\ny", FindQuery("x\ny"))
        assertEquals(m(0 to 3, 4 to 7), r.matches)
    }

    @Test
    fun cjkLiteralMatches() {
        val r = findAllMatches("你好 世界 你好", FindQuery("你好"))
        assertEquals(m(0 to 2, 6 to 8), r.matches)
    }

    @Test
    fun regexSpecialCharsAreLiteralWhenRegexOff() {
        val r = findAllMatches("a.c abc", FindQuery("a.c"))
        assertEquals(m(0 to 3), r.matches)
    }

    // --- 环绕导航 ----------------------------------------------------------------------------

    @Test
    fun nextFindsFirstMatchAtOrAfterOffset() {
        val ms = m(0 to 2, 5 to 7, 10 to 12)
        assertEquals(1, nextMatchIndex(ms, fromOffset = 3, forward = true))
        assertEquals(1, nextMatchIndex(ms, fromOffset = 5, forward = true))
    }

    @Test
    fun nextWrapsToStart() {
        val ms = m(0 to 2, 5 to 7)
        assertEquals(0, nextMatchIndex(ms, fromOffset = 8, forward = true))
    }

    @Test
    fun prevFindsLastMatchBeforeOffset() {
        val ms = m(0 to 2, 5 to 7, 10 to 12)
        assertEquals(1, nextMatchIndex(ms, fromOffset = 10, forward = false))
    }

    @Test
    fun prevWrapsToEnd() {
        val ms = m(0 to 2, 5 to 7)
        assertEquals(1, nextMatchIndex(ms, fromOffset = 0, forward = false))
    }

    @Test
    fun emptyMatchesNavigateToNull() {
        assertNull(nextMatchIndex(emptyList(), fromOffset = 0, forward = true))
        assertNull(nextMatchIndex(emptyList(), fromOffset = 0, forward = false))
    }
}
