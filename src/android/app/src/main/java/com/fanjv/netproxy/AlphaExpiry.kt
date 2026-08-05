package com.fanjv.netproxy

/** v8 Alpha 测试包的统一有效期判断。 */
internal object AlphaExpiry {
    fun isExpired(nowMillis: Long = System.currentTimeMillis()): Boolean =
        nowMillis >= BuildConfig.ALPHA_EXPIRES_AT_MILLIS
}
