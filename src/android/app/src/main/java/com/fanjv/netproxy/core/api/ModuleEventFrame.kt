package com.fanjv.netproxy.core.api

import kotlinx.serialization.KSerializer
import kotlinx.serialization.serializer
import kotlin.reflect.KType
import kotlin.reflect.typeOf

internal class ModuleEventFrame(private val content: String, private val codec: ModuleApiCodec) {
    private val decoded = HashMap<KType, Result<Any?>>()

    inline fun <reified T> decode(): T = decode(typeOf<T>(), serializer<T>())

    @Suppress("UNCHECKED_CAST")
    fun <T> decode(type: KType, serializer: KSerializer<T>): T = synchronized(decoded) {
        decoded.getOrPut(type) { runCatching { codec.decode(content, serializer) } }.getOrThrow() as T
    }
}

internal data class ModuleEventSnapshot(
    val frames: Map<String, ModuleEventFrame> = emptyMap(),
    val error: Throwable? = null,
)
