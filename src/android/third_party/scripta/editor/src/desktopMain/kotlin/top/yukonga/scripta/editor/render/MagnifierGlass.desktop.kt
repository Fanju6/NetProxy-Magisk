package top.yukonga.scripta.editor.render

import androidx.compose.ui.graphics.RenderEffect
import androidx.compose.ui.graphics.asComposeRenderEffect
import org.jetbrains.skia.ImageFilter
import org.jetbrains.skia.RuntimeEffect
import org.jetbrains.skia.RuntimeShaderBuilder

// SkSL 与 AGSL 同一份源码（[MAGNIFIER_GLASS_SHADER]）。RuntimeEffect 编译一次即缓存（避免每帧重编 SkSL）。
private val runtimeEffect: RuntimeEffect by lazy { RuntimeEffect.makeForShader(MAGNIFIER_GLASS_SHADER) }

actual fun magnifierGlassRenderEffect(
    left: Float,
    top: Float,
    width: Float,
    height: Float,
    cornerRadius: Float,
    refractionHeight: Float,
    refractionAmount: Float,
    depthEffect: Float,
    chromaticAberration: Float,
): RenderEffect? {
    if (width <= 0f || height <= 0f || refractionHeight <= 0f || refractionAmount <= 0f) return null
    val builder = RuntimeShaderBuilder(runtimeEffect)
    builder.uniform("size", width, height)
    builder.uniform("offset", -left, -top) // 使 centeredCoord = coord − 胶囊中心
    builder.uniform("cornerRadii", cornerRadius, cornerRadius, cornerRadius, cornerRadius)
    builder.uniform("refractionHeight", refractionHeight)
    builder.uniform("refractionAmount", -refractionAmount) // 负号：沿梯度反向（向内）折射，呈透镜边
    builder.uniform("depthEffect", depthEffect)
    builder.uniform("chromaticAberration", chromaticAberration)
    // input=null → 子 shader "content" 绑定滤镜源（作用图层自身内容）；与 Android createRuntimeShaderEffect 语义一致。
    return ImageFilter.makeRuntimeShader(builder, "content", null).asComposeRenderEffect()
}

actual fun isMagnifierGlassSupported(): Boolean = true
