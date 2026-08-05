package top.yukonga.scripta.editor.render

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/** [ScrollbarMath]：thumb 几何与行空间拖拽反解（softWrap 估算高度下行数是唯一稳定量）。 */
class ScrollbarMathTest {

    // --- thumbHeight ---

    @Test
    fun thumbHeightIsProportionalToVisibleFraction() {
        // viewport 1000、内容 4000（max=3000）→ 1000*1000/4000 = 250
        assertEquals(250f, ScrollbarMath.thumbHeight(1000f, 3000f, 48f))
    }

    @Test
    fun thumbHeightClampsToMinimum() {
        // 巨大内容：比例高远小于 minThumb → 钳到 48
        assertEquals(48f, ScrollbarMath.thumbHeight(1000f, 1_000_000f, 48f))
    }

    @Test
    fun noScrollMeansNoThumb() {
        assertEquals(0f, ScrollbarMath.thumbHeight(1000f, 0f, 48f))
        assertEquals(0f, ScrollbarMath.thumbHeight(1000f, -5f, 48f))
    }

    @Test
    fun tinyViewportSmallerThanMinThumbHidesThumb() {
        // viewport 40 < minThumb 48：钳到 viewport 后轨道无余量 → 不显示（反解会除零/负）
        assertEquals(0f, ScrollbarMath.thumbHeight(40f, 500f, 48f))
    }

    // --- thumbTop ---

    @Test
    fun thumbTopTracksScrollFraction() {
        val h = ScrollbarMath.thumbHeight(1000f, 3000f, 48f) // 250
        assertEquals(0f, ScrollbarMath.thumbTop(1000f, 3000f, h, 0f))
        assertEquals(750f, ScrollbarMath.thumbTop(1000f, 3000f, h, 3000f)) // 底：viewport-thumbH
        assertEquals(375f, ScrollbarMath.thumbTop(1000f, 3000f, h, 1500f)) // 中点
    }

    @Test
    fun thumbTopClampsOverscrolledValues() {
        val h = 250f
        assertEquals(750f, ScrollbarMath.thumbTop(1000f, 3000f, h, 9999f))
        assertEquals(0f, ScrollbarMath.thumbTop(1000f, 3000f, h, -10f))
    }

    // --- dragTargetLine（行空间反解）---

    @Test
    fun dragByFullTrackSpansWholeDocument() {
        // 轨道 = 1000-250 = 750；从行 0 拖满轨道 → 末行
        val target = ScrollbarMath.dragTargetLine(
            grabLine = 0, downY = 100f, nowY = 850f, viewport = 1000f, thumbH = 250f, lineCount = 240_000,
        )
        assertEquals(239_999, target)
    }

    @Test
    fun dragDeltaIsAnchoredToGrabLine() {
        // 从行 1000 抓起、向上拖 75px（轨道 750 的 10%）→ 目标 = 1000 - 24000 → 钳 0
        val up = ScrollbarMath.dragTargetLine(1000, 500f, 425f, 1000f, 250f, 240_000)
        assertEquals(0, up)
        // 向下 75px → 1000 + 10% * 239999 ≈ 24999
        val down = ScrollbarMath.dragTargetLine(1000, 500f, 575f, 1000f, 250f, 240_000)
        assertTrue(down in 24_990..25_010)
    }

    @Test
    fun zeroDeltaStaysOnGrabLine() {
        assertEquals(1234, ScrollbarMath.dragTargetLine(1234, 500f, 500f, 1000f, 250f, 240_000))
    }

    @Test
    fun dragClampsToDocumentEnds() {
        assertEquals(0, ScrollbarMath.dragTargetLine(5, 500f, -9999f, 1000f, 250f, 100))
        assertEquals(99, ScrollbarMath.dragTargetLine(5, 500f, 9999f, 1000f, 250f, 100))
    }

    @Test
    fun degenerateTrackOrSingleLineIsSafe() {
        assertEquals(0, ScrollbarMath.dragTargetLine(0, 0f, 100f, 100f, 100f, 50)) // 轨道 0
        assertEquals(0, ScrollbarMath.dragTargetLine(0, 0f, 100f, 1000f, 250f, 1)) // 单行
    }

    // --- hitThumb ---

    @Test
    fun hitRequiresRightEdgeHotZoneAndThumbBand() {
        // viewport 宽 800、热区 60、thumb [200, 450]、纵向余量 20
        assertTrue(ScrollbarMath.hitThumb(790f, 300f, 800f, 60f, 200f, 250f, 20f))
        assertTrue(ScrollbarMath.hitThumb(741f, 185f, 800f, 60f, 200f, 250f, 20f)) // 热区左界 + 上余量内
        assertTrue(!ScrollbarMath.hitThumb(700f, 300f, 800f, 60f, 200f, 250f, 20f)) // x 在热区外
        assertTrue(!ScrollbarMath.hitThumb(790f, 100f, 800f, 60f, 200f, 250f, 20f)) // y 在 thumb 带外
        assertTrue(!ScrollbarMath.hitThumb(790f, 500f, 800f, 60f, 200f, 250f, 20f))
    }
}
