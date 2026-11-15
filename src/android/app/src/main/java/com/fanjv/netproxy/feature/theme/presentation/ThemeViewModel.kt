package com.fanjv.netproxy.feature.theme.presentation

import androidx.lifecycle.ViewModel
import kotlinx.coroutines.flow.StateFlow

/** 管理仅属于 Android 客户端的主题与界面偏好。 */
internal class ThemeViewModel(
    private val manager: ThemeManager
) : ViewModel() {

    val state: StateFlow<ThemeUiState> = manager.state

    fun setMiuixMonet(enabled: Boolean) = manager.setMiuixMonet(enabled)
    fun setKeyColor(color: Int) = manager.setKeyColor(color)
    fun setColorStyle(style: String) = manager.setColorStyle(style)
    fun setColorSpec(spec: String) = manager.setColorSpec(spec)
    fun setPageScale(scale: Float) = manager.setPageScale(scale)
}

