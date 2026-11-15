package com.fanjv.netproxy.feature.theme.presentation

import android.content.SharedPreferences
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update

/**
 * 持有主题/外观状态及其持久化，可用伪造的 [prefs] 进行单元测试。
 *
 * 状态在构造时同步从 SharedPreferences 读取，确保首帧合成（为整个应用着色）
 * 即可看到已持久化的值。
 */
class ThemeManager(prefs: SharedPreferences) {
    private val store = ThemePreferenceStore(prefs)

    private val _state: MutableStateFlow<ThemeUiState>
    val state: StateFlow<ThemeUiState>

    init {
        val p = store.read()
        _state = MutableStateFlow(
            ThemeUiState(
                miuixMonet = p.miuixMonet,
                keyColor = p.keyColor,
                colorStyle = p.colorStyle,
                colorSpec = p.colorSpec,
                pageScale = p.pageScale,
            )
        )
        state = _state.asStateFlow()
    }

    fun setMiuixMonet(enabled: Boolean) {
        store.setMiuixMonet(enabled)
        _state.update { it.copy(miuixMonet = enabled) }
    }

    fun setKeyColor(color: Int) {
        store.setKeyColor(color)
        _state.update { it.copy(keyColor = color) }
    }

    fun setColorStyle(style: String) {
        store.setColorStyle(style)
        _state.update { it.copy(colorStyle = style) }
    }

    fun setColorSpec(spec: String) {
        store.setColorSpec(spec)
        _state.update { it.copy(colorSpec = spec) }
    }

    fun setPageScale(scale: Float) {
        store.setPageScale(scale)
        _state.update { it.copy(pageScale = scale) }
    }
}

