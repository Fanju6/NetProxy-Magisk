package com.fanjv.netproxy.feature.about.presentation.effect

internal enum class AboutDeviceType {
    Phone,
    Pad,
}

internal object AboutBackgroundConfig {
    internal class Preset(
        val points: FloatArray,
        val colors1: FloatArray,
        val colors2: FloatArray,
        val colors3: FloatArray,
        val colorInterpPeriod: Float,
        val lightOffset: Float,
        val saturateOffset: Float,
        val pointOffset: Float,
    )

    private val phoneLight = Preset(
        points = floatArrayOf(0.8f, 0.2f, 1f, 0.8f, 0.9f, 1f, 0.2f, 0.9f, 1f, 0.2f, 0.2f, 1f),
        colors1 = floatArrayOf(1f, 0.9f, 0.94f, 1f, 1f, 0.84f, 0.89f, 1f, 0.97f, 0.73f, 0.82f, 1f, 0.64f, 0.65f, 0.98f, 1f),
        colors2 = floatArrayOf(0.58f, 0.74f, 1f, 1f, 1f, 0.9f, 0.93f, 1f, 0.74f, 0.76f, 1f, 1f, 0.97f, 0.77f, 0.84f, 1f),
        colors3 = floatArrayOf(0.98f, 0.86f, 0.9f, 1f, 0.6f, 0.73f, 0.98f, 1f, 0.92f, 0.93f, 1f, 1f, 0.56f, 0.69f, 1f, 1f),
        colorInterpPeriod = 5f,
        lightOffset = 0.1f,
        saturateOffset = 0.2f,
        pointOffset = 0.2f,
    )

    private val phoneDark = Preset(
        points = floatArrayOf(0.8f, 0.2f, 1f, 0.8f, 0.9f, 1f, 0.2f, 0.9f, 1f, 0.2f, 0.2f, 1f),
        colors1 = floatArrayOf(0.2f, 0.06f, 0.88f, 0.4f, 0.3f, 0.14f, 0.55f, 0.5f, 0f, 0.64f, 0.96f, 0.5f, 0.11f, 0.16f, 0.83f, 0.4f),
        colors2 = floatArrayOf(0.07f, 0.15f, 0.79f, 0.5f, 0.62f, 0.21f, 0.67f, 0.5f, 0.06f, 0.25f, 0.84f, 0.5f, 0f, 0.2f, 0.78f, 0.5f),
        colors3 = floatArrayOf(0.58f, 0.3f, 0.74f, 0.4f, 0.27f, 0.18f, 0.6f, 0.5f, 0.66f, 0.26f, 0.62f, 0.5f, 0.12f, 0.16f, 0.7f, 0.6f),
        colorInterpPeriod = 8f,
        lightOffset = 0f,
        saturateOffset = 0.17f,
        pointOffset = 0.4f,
    )

    private val padLight = Preset(
        points = floatArrayOf(0.8f, 0.2f, 1f, 0.8f, 0.9f, 1f, 0.2f, 0.9f, 1f, 0.2f, 0.2f, 1f),
        colors1 = floatArrayOf(0.99f, 0.77f, 0.86f, 1f, 0.74f, 0.76f, 1f, 1f, 0.72f, 0.74f, 1f, 1f, 0.98f, 0.76f, 0.8f, 1f),
        colors2 = floatArrayOf(0.66f, 0.75f, 1f, 1f, 1f, 0.86f, 0.91f, 1f, 0.74f, 0.76f, 1f, 1f, 0.97f, 0.77f, 0.84f, 1f),
        colors3 = floatArrayOf(0.97f, 0.79f, 0.85f, 1f, 0.65f, 0.68f, 0.98f, 1f, 0.66f, 0.77f, 1f, 1f, 0.72f, 0.73f, 0.98f, 1f),
        colorInterpPeriod = 7f,
        lightOffset = 0.1f,
        saturateOffset = 0.2f,
        pointOffset = 0.2f,
    )

    private val padDark = Preset(
        points = floatArrayOf(0.8f, 0.2f, 1f, 0.8f, 0.9f, 1f, 0.2f, 0.9f, 1f, 0.2f, 0.2f, 1f),
        colors1 = floatArrayOf(0.66f, 0.26f, 0.62f, 0.4f, 0.06f, 0.25f, 0.84f, 0.5f, 0f, 0.64f, 0.96f, 0.5f, 0.14f, 0.18f, 0.55f, 0.5f),
        colors2 = floatArrayOf(0.07f, 0.15f, 0.79f, 0.5f, 0.11f, 0.16f, 0.83f, 0.5f, 0.06f, 0.25f, 0.84f, 0.5f, 0.66f, 0.26f, 0.62f, 0.5f),
        colors3 = floatArrayOf(0.58f, 0.3f, 0.74f, 0.5f, 0.11f, 0.16f, 0.83f, 0.5f, 0.66f, 0.26f, 0.62f, 0.5f, 0.27f, 0.18f, 0.6f, 0.6f),
        colorInterpPeriod = 7f,
        lightOffset = 0f,
        saturateOffset = 0f,
        pointOffset = 0.2f,
    )

    internal fun get(deviceType: AboutDeviceType, darkTheme: Boolean): Preset =
        when (deviceType) {
            AboutDeviceType.Phone -> if (darkTheme) phoneDark else phoneLight
            AboutDeviceType.Pad -> if (darkTheme) padDark else padLight
        }
}
