package top.yukonga.scripta.editor.highlight

import kotlin.test.Test
import kotlin.test.assertEquals

class HighlightCacheTest {

    private fun cacheOver(lines: MutableList<String>): Pair<HighlightCache, (Int) -> String> {
        val cache = HighlightCache(YamlHighlighter())
        return cache to { i: Int -> lines[i] }
    }

    @Test
    fun statePropagatesAcrossLines() {
        val lines = mutableListOf("a: |", "  body", "b: 1")
        val (cache, get) = cacheOver(lines)
        // 行 1 在块标量体内 → 整行字符串
        assertEquals(listOf(HighlightSpan(2, 6, TokenType.String)), cache.spansForLine(1, get))
        // 行 2 缩进回落 → 正常键值
        assertEquals(TokenType.Key, cache.spansForLine(2, get)[0].type)
    }

    @Test
    fun randomAccessComputesPrecedingStates() {
        val lines = mutableListOf("a: |", "  x", "  y", "b: 2")
        val (cache, get) = cacheOver(lines)
        // 直接请求行 2（未先请求 0/1）也要得到块标量体
        assertEquals(TokenType.String, cache.spansForLine(2, get).single().type)
    }

    @Test
    fun invalidateFromRecomputesDownstream() {
        val lines = mutableListOf("a: |", "  x: y", "b: 1")
        val (cache, get) = cacheOver(lines)
        // 初始：行 1 是块标量体 → 单一 String 段
        assertEquals(1, cache.spansForLine(1, get).size)
        // 把行 0 改成普通键值：块标量消失，行 1 变成缩进的键值对
        lines[0] = "a: 1"
        cache.invalidateFrom(0)
        val after = cache.spansForLine(1, get)
        assertEquals(TokenType.Key, after[0].type)
    }

    @Test
    fun invalidateBelowRequestedLineDoesNotAffectAbove() {
        val lines = mutableListOf("a: 1", "b: 2", "c: 3")
        val (cache, get) = cacheOver(lines)
        cache.spansForLine(2, get)
        cache.invalidateFrom(1)
        // 行 0 状态仍可信，重新请求行 0/2 都正确
        assertEquals(TokenType.Key, cache.spansForLine(0, get)[0].type)
        assertEquals(TokenType.Key, cache.spansForLine(2, get)[0].type)
    }
}
