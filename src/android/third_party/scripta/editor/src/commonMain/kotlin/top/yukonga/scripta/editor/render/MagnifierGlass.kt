package top.yukonga.scripta.editor.render

import androidx.compose.ui.graphics.RenderEffect

/**
 * 圆角矩形「液态玻璃」边缘 [RenderEffect]：对已渲染内容做**折射 + 色散**（无模糊）。作用于放大镜**自身**的放大内容（非背景
 * backdrop）——near 胶囊边缘把放大文本折射弯曲、并在 rim 产生彩色色散，营造玻璃透镜边。
 *
 * shader（[MAGNIFIER_GLASS_SHADER]）改编自 Kyant0/AndroidLiquidGlass（Apache-2.0，与 miuix 的 `lens` 同源）。AGSL 与 SkSL
 * 同一份源码：Android 13+ 走 `RuntimeShader`/`createRuntimeShaderEffect`，桌面(skiko) 走 `RuntimeEffect`/`ImageFilter.makeRuntimeShader`。
 * 仅 **Android < 13**（无 AGSL）返回 `null`，放大镜退化为普通圆角边（见各 actual 与 [isMagnifierGlassSupported]）。
 *
 * 坐标系为「作用图层的像素空间」：[left]/[top]/[width]/[height] 为胶囊在该图层内的矩形；[cornerRadius] 角半径；
 * [refractionHeight] 折射带自边缘向内的深度；[refractionAmount] 边缘最大折射位移；[depthEffect] 折射方向向「从中心的径向」
 * 混合的强度（0=纯边缘法线折射/平、1=域向、鼓起玻璃感；[chromaticAberration] 色散强度（0=无、~0.1 轻微、~0.2 明显）。
 * 返回 `null` 表示当前平台/参数不产生效果。
 */
expect fun magnifierGlassRenderEffect(
    left: Float,
    top: Float,
    width: Float,
    height: Float,
    cornerRadius: Float,
    refractionHeight: Float,
    refractionAmount: Float,
    depthEffect: Float,
    chromaticAberration: Float,
): RenderEffect?

/**
 * 当前平台是否支持放大镜液态玻璃边（= [magnifierGlassRenderEffect] 会产生效果）：桌面(skiko) 恒 true、Android 13+ true、
 * 更低 Android false。支持时玻璃 rim 即为边缘、不再画硬描边；不支持时退化画一圈普通描边界定胶囊（否则胶囊底色与正文同色、
 * 只剩阴影、几乎看不出边）。
 */
expect fun isMagnifierGlassSupported(): Boolean

/**
 * 折射 + 色散 shader 源（AGSL/SkSL 通用）。改编自 Kyant0/AndroidLiquidGlass（Apache-2.0），与 miuix 的 `lens` 同源。**无模糊**。
 * 相对原版加了一处优化：`sd > 0`（胶囊外像素）直接直通，避免在全屏图层上对每个外部像素都跑 7 次色散采样。
 */
internal const val MAGNIFIER_GLASS_SHADER = """
uniform shader content;
uniform float2 size;
uniform float2 offset;
uniform float4 cornerRadii;
uniform float refractionHeight;
uniform float refractionAmount;
uniform float depthEffect;
uniform float chromaticAberration;

float radiusAt(float2 coord, float4 radii) {
    if (coord.x >= 0.0) {
        if (coord.y <= 0.0) return radii.y; else return radii.z;
    } else {
        if (coord.y <= 0.0) return radii.x; else return radii.w;
    }
}

float sdRoundedRect(float2 coord, float2 halfSize, float radius) {
    float2 cornerCoord = abs(coord) - (halfSize - float2(radius));
    float outside = length(max(cornerCoord, 0.0)) - radius;
    float inside = min(max(cornerCoord.x, cornerCoord.y), 0.0);
    return outside + inside;
}

float2 gradSdRoundedRect(float2 coord, float2 halfSize, float radius) {
    float2 cornerCoord = abs(coord) - (halfSize - float2(radius));
    if (cornerCoord.x >= 0.0 || cornerCoord.y >= 0.0) {
        return sign(coord) * normalize(max(cornerCoord, 0.0));
    } else {
        float gradX = step(cornerCoord.y, cornerCoord.x);
        return sign(coord) * float2(gradX, 1.0 - gradX);
    }
}

float circleMap(float x) { return 1.0 - sqrt(1.0 - x * x); }

half4 main(float2 coord) {
    float2 halfSize = size * 0.5;
    float2 centeredCoord = (coord + offset) - halfSize;
    float radius = radiusAt(centeredCoord, cornerRadii);

    float sd = sdRoundedRect(centeredCoord, halfSize, radius);
    if (sd > 0.0) {
        return content.eval(coord);
    }
    if (-sd >= refractionHeight) {
        return content.eval(coord);
    }

    float d = circleMap(1.0 - -sd / refractionHeight) * refractionAmount;
    float gradRadius = min(radius * 1.5, min(halfSize.x, halfSize.y));
    float2 grad = normalize(gradSdRoundedRect(centeredCoord, halfSize, gradRadius) + depthEffect * normalize(centeredCoord));

    float2 refractedCoord = coord + d * grad;
    float dispersionIntensity = chromaticAberration * ((centeredCoord.x * centeredCoord.y) / (halfSize.x * halfSize.y));
    float2 dispersedCoord = d * grad * dispersionIntensity;

    half4 color = half4(0.0);

    half4 red = content.eval(refractedCoord + dispersedCoord);
    color.r += red.r / 3.5;
    color.a += red.a / 7.0;

    half4 orange = content.eval(refractedCoord + dispersedCoord * (2.0 / 3.0));
    color.r += orange.r / 3.5;
    color.g += orange.g / 7.0;
    color.a += orange.a / 7.0;

    half4 yellow = content.eval(refractedCoord + dispersedCoord * (1.0 / 3.0));
    color.r += yellow.r / 3.5;
    color.g += yellow.g / 3.5;
    color.a += yellow.a / 7.0;

    half4 green = content.eval(refractedCoord);
    color.g += green.g / 3.5;
    color.a += green.a / 7.0;

    half4 cyan = content.eval(refractedCoord - dispersedCoord * (1.0 / 3.0));
    color.g += cyan.g / 3.5;
    color.b += cyan.b / 3.0;
    color.a += cyan.a / 7.0;

    half4 blue = content.eval(refractedCoord - dispersedCoord * (2.0 / 3.0));
    color.b += blue.b / 3.0;
    color.a += blue.a / 7.0;

    half4 purple = content.eval(refractedCoord - dispersedCoord);
    color.r += purple.r / 7.0;
    color.b += purple.b / 3.0;
    color.a += purple.a / 7.0;

    return color;
}
"""
