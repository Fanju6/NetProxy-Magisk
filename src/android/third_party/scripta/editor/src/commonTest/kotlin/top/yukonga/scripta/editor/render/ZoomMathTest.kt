package top.yukonga.scripta.editor.render

import kotlin.test.Test
import kotlin.test.assertEquals

class ZoomMathTest {
    @Test
    fun clampScaleKeepsFontInRange() {
        assertEquals(40f / 14f, ZoomMath.clampScaleToFontRange(10f, 14f, 8f, 40f), 1e-4f) // 放大撞上界
        assertEquals(8f / 14f, ZoomMath.clampScaleToFontRange(0.1f, 14f, 8f, 40f), 1e-4f) // 缩小撞下界
        assertEquals(1.5f, ZoomMath.clampScaleToFontRange(1.5f, 14f, 8f, 40f), 1e-4f)      // 区间内原样
        assertEquals(2f, ZoomMath.clampScaleToFontRange(2f, 0f, 8f, 40f), 1e-4f)           // fontStart<=0 原样
    }

    @Test
    fun commitFontSizeIsExactAndClamps() {
        assertEquals(15.4f, ZoomMath.commitFontSize(14f, 1.1f, 8f, 40f), 1e-4f) // 精确、不量化到档
        assertEquals(14f, ZoomMath.commitFontSize(14f, 1f, 8f, 40f), 1e-4f)     // 系数 1 不变
        assertEquals(40f, ZoomMath.commitFontSize(14f, 100f, 8f, 40f), 1e-4f)   // 上钳
        assertEquals(8f, ZoomMath.commitFontSize(14f, 0.01f, 8f, 40f), 1e-4f)   // 下钳
    }

    @Test
    fun provisionalWrapReturnsFocalToViewportY() {
        assertEquals(200f - 60f, ZoomMath.provisionalScrollYWrap(200f, 1f, 60f), 1e-4f)
        assertEquals(300f * 2f - 60f, ZoomMath.provisionalScrollYWrap(300f, 2f, 60f), 1e-4f)
    }
}
