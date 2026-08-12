package com.fanjv.netproxy.core.ui

import androidx.annotation.StringRes
import androidx.compose.runtime.Composable
import androidx.compose.ui.res.stringResource

/** 延迟到界面层解析的用户可见文案，避免 ViewModel 固定写入本地化文本。 */
internal sealed interface UiText {
    data object Empty : UiText

    data class Plain(val value: String) : UiText

    data class Resource(
        @StringRes val id: Int,
        val args: List<Any> = emptyList()
    ) : UiText
}

internal class UiTextException(val text: UiText) : IllegalArgumentException()

internal fun String.toUiText(): UiText = UiText.Plain(this)

internal fun Throwable.toUiText(): UiText = when (this) {
    is UiTextException -> text
    else -> userMessage().toUiText()
}

@Composable
internal fun UiText.resolve(): String = when (this) {
    UiText.Empty -> ""
    is UiText.Plain -> value
    is UiText.Resource -> stringResource(id, *args.toTypedArray())
}
