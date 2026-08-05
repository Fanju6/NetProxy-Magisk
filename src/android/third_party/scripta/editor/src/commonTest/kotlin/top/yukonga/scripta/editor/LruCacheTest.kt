package top.yukonga.scripta.editor

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

class LruCacheTest {
    @Test
    fun evictsLeastRecentlyUsedOnOverflow() {
        val c = LruCache<Int, String>(2)
        c[1] = "a"; c[2] = "b"
        c[3] = "c" // 超容量 -> 淘汰最久未用的 1
        assertNull(c[1])
        assertEquals("b", c[2])
        assertEquals("c", c[3])
        assertEquals(2, c.size)
    }

    @Test
    fun getBumpsRecencySoVisibleKeySurvives() {
        val c = LruCache<Int, String>(2)
        c[1] = "a"; c[2] = "b"
        assertEquals("a", c[1]) // 访问 1 -> 1 成为最近使用
        c[3] = "c" // 淘汰此时最久未用的 2，而非 1
        assertEquals("a", c[1])
        assertNull(c[2])
        assertEquals("c", c[3])
    }

    @Test
    fun updateExistingBumpsAndKeepsSize() {
        val c = LruCache<Int, String>(2)
        c[1] = "a"; c[2] = "b"
        c[1] = "A" // 更新 1 -> 最近使用
        c[3] = "c" // 淘汰 2
        assertEquals("A", c[1])
        assertNull(c[2])
        assertEquals(2, c.size)
    }

    @Test
    fun getOrPutComputesOnceThenHits() {
        val c = LruCache<Int, String>(4)
        var calls = 0
        val a = c.getOrPut(1) { calls++; "x" }
        val b = c.getOrPut(1) { calls++; "y" }
        assertEquals("x", a)
        assertEquals("x", b) // 命中缓存，不重算
        assertEquals(1, calls)
    }
}
