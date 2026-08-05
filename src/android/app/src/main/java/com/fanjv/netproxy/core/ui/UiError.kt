package com.fanjv.netproxy.core.ui

import kotlinx.coroutines.CancellationException

/** 将业务异常转为界面文案，同时保持协程取消语义。 */
internal fun Throwable.userMessage(): String {
    if (this is CancellationException) throw this
    return message?.takeIf(String::isNotBlank) ?: "操作失败，请稍后重试"
}
