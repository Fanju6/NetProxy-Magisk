package top.yukonga.scripta.editor.render

/**
 * 双指缩放用的纯数学助手，与 Compose 无关、便于单测。手势期「四方向自由跟手」的预览变换现由 CodeEditor/EditorCanvas
 * 在 draw 阶段内联完成（自由增量仿射 + 就地钳制）；这里只留提交 / 边界用的三个纯函数：
 * - [clampScaleToFontRange]：把连续缩放系数钳到字号落 [minSp,maxSp] 的范围（手势中撞界即不再继续）；
 * - [commitFontSize]：松手把连续目标字号精确落成真实字号（不量化到档）；
 * - [provisionalScrollYWrap]：软换行提交那一帧的同步临时 scrollY 锚定。
 */
object ZoomMath {
    /**
     * 把连续缩放系数 [scale]（相对手势起始字号 [fontStart]）限制在字号落 [minSp,maxSp] 的范围内，
     * 使手势中放大/缩小到边界后不再继续（预览与最终字号一致）。[fontStart] 非正时原样返回。
     */
    fun clampScaleToFontRange(scale: Float, fontStart: Float, minSp: Float, maxSp: Float): Float {
        if (fontStart <= 0f) return scale
        return scale.coerceIn(minSp / fontStart, maxSp / fontStart)
    }

    /**
     * 提交缩放的最终字号：连续目标字号 `fontStart*scale` 钳到 [minSp,maxSp]。**不量化到档**——只在松手重排一次、无逐档
     * 性能压力；提交精确目标使内容/gutter 与预览末帧完全连续、消除提交时的字号吸附跳变（配合自相似布局，提交零跳变）。
     */
    fun commitFontSize(fontStart: Float, scale: Float, minSp: Float, maxSp: Float): Float =
        (fontStart * scale).coerceIn(minSp, maxSp)

    /**
     * 换行提交那一帧的同步临时 scrollY：把锚字符旧内容 y（[anchorContentYOld]）按 k 缩放再减目标视口 y（[anchorViewportY]，
     * 顶部锚为 0），使提交帧≈预览末帧（锚点仍在目标 y 附近）、避免单帧 lurch；随后由 post-layout 效应按新布局精确校正一次。
     */
    fun provisionalScrollYWrap(anchorContentYOld: Float, k: Float, anchorViewportY: Float): Float =
        (anchorContentYOld * k - anchorViewportY).coerceAtLeast(0f)
}
