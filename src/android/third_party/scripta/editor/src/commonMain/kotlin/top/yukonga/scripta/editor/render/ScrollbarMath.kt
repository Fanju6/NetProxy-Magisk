package top.yukonga.scripta.editor.render

/**
 * 滚动条几何：thumb 高/位与命中是像素比例；**拖拽反解在文档行空间**——softWrap 下未测量行按 1 视觉行
 * 估算，thumb 拖入新区域时可见测量会逐帧抬高内容高/maxScroll，像素反解会让 thumb 从指下溜走、拖到
 * 轨道底也够不到真实文末；行数是唯一稳定量，按「抓取行 + 轨道分数位移 × 行数」求目标行则拖满轨道
 * 必达末行。全部纯函数。
 */
object ScrollbarMath {

    /** thumb 高：可见比例 × 轨道，钳到 [minThumb, viewport]。返回 0 = 不显示（无可滚、或视口小到
     *  钳完没有轨道余量——那种情况反解要除以 (viewport − thumbH)，必须先在此挡掉）。 */
    fun thumbHeight(viewport: Float, maxScroll: Float, minThumb: Float): Float {
        if (maxScroll <= 0f || viewport <= 0f) return 0f
        val content = viewport + maxScroll
        val h = (viewport * viewport / content).coerceAtLeast(minThumb).coerceAtMost(viewport)
        return if (viewport - h <= 0f) 0f else h
    }

    /** thumb 顶：轨道 (viewport − thumbH) 内按滚动比例落位；越界滚动量夹回。 */
    fun thumbTop(viewport: Float, maxScroll: Float, thumbH: Float, scroll: Float): Float =
        if (maxScroll <= 0f) 0f else (viewport - thumbH) * (scroll / maxScroll).coerceIn(0f, 1f)

    /** 拖拽反解（行空间）：抓取行锚定 + 位移的轨道分数 × (行数−1)，钳到文档行界。
     *  delta 锚定（而非绝对位置映射）使抓取瞬间零跳变——抓哪儿从哪儿算。 */
    fun dragTargetLine(grabLine: Int, downY: Float, nowY: Float, viewport: Float, thumbH: Float, lineCount: Int): Int {
        val last = (lineCount - 1).coerceAtLeast(0)
        val track = viewport - thumbH
        if (track <= 0f || last == 0) return grabLine.coerceIn(0, last)
        val deltaLines = ((nowY - downY) / track * last).toInt()
        return (grabLine + deltaLines).coerceIn(0, last)
    }

    /** 纵条可抓命中：x 落右缘热区（宽 [hotWidth]）且 y 落 thumb 带 ±[slack]。 */
    fun hitThumb(x: Float, y: Float, viewportWidth: Float, hotWidth: Float, thumbTop: Float, thumbH: Float, slack: Float): Boolean =
        x >= viewportWidth - hotWidth && y >= thumbTop - slack && y <= thumbTop + thumbH + slack
}
