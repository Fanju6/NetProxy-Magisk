package com.fanjv.netproxy.core.api

import kotlinx.serialization.ExperimentalSerializationApi
import kotlinx.serialization.KSerializer
import kotlinx.serialization.Serializable
import kotlinx.serialization.SerializationException
import kotlinx.serialization.descriptors.PrimitiveKind
import kotlinx.serialization.descriptors.PrimitiveSerialDescriptor
import kotlinx.serialization.encoding.Decoder
import kotlinx.serialization.encoding.Encoder
import kotlinx.serialization.json.*
import java.io.InputStream

@Serializable
internal data class ModuleEnvelope<T>(
    // 仅用无效哨兵识别缺失字段；检查时拒绝，不作为协议默认值。
    @Serializable(with = ModuleSchemaSerializer::class) val schema: Int = 0,
    @Serializable(with = ModuleBooleanSerializer::class) val ok: Boolean,
    @Serializable(with = ModuleStringSerializer::class) val code: String,
    @Serializable(with = ModuleStringSerializer::class) val message: String,
    val data: T,
)

internal object ModuleSchemaSerializer : KSerializer<Int> {
    override val descriptor = PrimitiveSerialDescriptor("ModuleSchema", PrimitiveKind.INT)
    override fun serialize(encoder: Encoder, value: Int) = encoder.encodeInt(value)
    override fun deserialize(decoder: Decoder): Int {
        val value = (decoder as JsonDecoder).decodeJsonElement() as? JsonPrimitive
        if (value == null || value.isString || value.intOrNull != 1) {
            throw ModuleApiException("api.unsupported_schema", "模块管理接口版本不受支持")
        }
        return 1
    }
}

internal object ModuleBooleanSerializer : KSerializer<Boolean> {
    override val descriptor = PrimitiveSerialDescriptor("ModuleBoolean", PrimitiveKind.BOOLEAN)
    override fun serialize(encoder: Encoder, value: Boolean) = encoder.encodeBoolean(value)
    override fun deserialize(decoder: Decoder): Boolean {
        val value = (decoder as JsonDecoder).decodeJsonElement() as? JsonPrimitive
        return value?.takeUnless { it.isString }?.booleanOrNull ?: throw SerializationException("布尔字段类型无效")
    }
}

internal object ModuleStringSerializer : KSerializer<String> {
    override val descriptor = PrimitiveSerialDescriptor("ModuleString", PrimitiveKind.STRING)
    override fun serialize(encoder: Encoder, value: String) = encoder.encodeString(value)
    override fun deserialize(decoder: Decoder): String {
        val value = (decoder as JsonDecoder).decodeJsonElement() as? JsonPrimitive
        return value?.takeIf { it.isString }?.content ?: throw SerializationException("字符串字段类型无效")
    }
}

@OptIn(ExperimentalSerializationApi::class)
internal class ModuleApiCodec(private val json: Json) {
    fun decode(status: Int, input: InputStream): ModuleApiResponse {
        val root = try {
            json.decodeFromStream(JsonObject.serializer(), input)
        } catch (_: SerializationException) {
            throw ModuleApiException("api.invalid_response", "模块返回的数据格式无效")
        }
        return decode(status, root)
    }

    fun <T> decode(status: Int, input: InputStream, serializer: KSerializer<T>): T {
        if (status !in 200..299) {
            decode(status, input)
            error("HTTP 错误未传播")
        }
        try {
            return checked(json.decodeFromStream(ModuleEnvelope.serializer(serializer), input))
        } catch (error: SerializationException) {
            throw ModuleApiException("api.invalid_response", "模块返回的数据格式无效")
        }
    }

    fun <T> decode(content: String, serializer: KSerializer<T>): T = try {
        checked(json.decodeFromString(ModuleEnvelope.serializer(serializer), content))
    } catch (error: SerializationException) {
        decode(200, content)
        throw ModuleApiException("api.invalid_response", "模块返回的数据格式无效")
    } catch (error: ModuleApiException) {
        decode(200, content)
        throw error
    }

    private fun <T> checked(envelope: ModuleEnvelope<T>): T {
        if (envelope.schema != 1) throw ModuleApiException("api.unsupported_schema", "模块管理接口版本不受支持")
        if (envelope.code.isEmpty()) throw ModuleApiException("api.invalid_response", "模块返回的数据格式无效")
        if (!envelope.ok) throw ModuleApiException(envelope.code, envelope.message.ifEmpty { "模块操作失败" })
        return envelope.data
    }

    fun decode(status: Int, content: String): ModuleApiResponse {
        val root = runCatching { json.parseToJsonElement(content).jsonObject }.getOrElse {
            throw ModuleApiException("api.invalid_response", "模块返回的数据格式无效")
        }
        return decode(status, root)
    }

    private fun decode(status: Int, root: JsonObject): ModuleApiResponse {
        val schema = root["schema"] as? JsonPrimitive
        if (schema == null || schema.isString || schema.intOrNull != 1) throw ModuleApiException("api.unsupported_schema", "模块管理接口版本不受支持")
        val ok = root["ok"] as? JsonPrimitive
        val code = (root["code"] as? JsonPrimitive)?.takeIf { it.isString }?.content
        val message = (root["message"] as? JsonPrimitive)?.takeIf { it.isString }?.content
        if (ok == null || ok.isString || ok.booleanOrNull == null || code.isNullOrEmpty() || message == null) {
            throw ModuleApiException("api.invalid_response", "模块返回的数据格式无效")
        }
        val data = root["data"] ?: JsonObject(emptyMap())
        if (status !in 200..299 || ok.booleanOrNull != true) throw ModuleApiException(code, message.ifEmpty { "模块操作失败" }, data)
        return ModuleApiResponse(code, message, data)
    }
}
