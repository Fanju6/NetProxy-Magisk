package com.fanjv.netproxy.core.api

import kotlinx.coroutines.runBlocking
import kotlinx.serialization.KSerializer
import kotlinx.serialization.Serializable
import kotlinx.serialization.encoding.Decoder
import kotlinx.serialization.encoding.Encoder
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.decodeFromStream
import kotlinx.serialization.serializer
import org.junit.Assert.*
import org.junit.Test
import java.io.ByteArrayInputStream
import java.lang.management.ManagementFactory
import com.sun.management.ThreadMXBean
import kotlin.reflect.typeOf

class ModuleApiCodecTest {
    private val codec = ModuleApiCodec(Json { ignoreUnknownKeys = true })

    @Serializable
    internal data class Payload(val value: Int)

    @Test fun typedReadDecodesTheResponseStreamDirectly() = runBlocking {
        val client = ModuleApiClient(TestApiTransport { apiSuccess("""{"value":42,"future":true}""") })
        assertEquals(Payload(42), client.read<Payload>("/fixture"))
    }

    @Test fun typedHeadersKeepStrictPrimitiveTypesAndSchemaErrors() {
        for ((header, code) in listOf(
            "\"schema\":\"1\",\"ok\":true,\"code\":\"ok\",\"message\":\"\"" to "api.unsupported_schema",
            "\"schema\":2,\"ok\":true,\"code\":\"ok\",\"message\":\"\"" to "api.unsupported_schema",
            "\"ok\":true,\"code\":\"ok\",\"message\":\"\"" to "api.unsupported_schema",
            "\"schema\":1,\"ok\":\"true\",\"code\":\"ok\",\"message\":\"\"" to "api.invalid_response",
            "\"schema\":1,\"ok\":true,\"code\":23,\"message\":\"\"" to "api.invalid_response",
            "\"schema\":1,\"ok\":true,\"code\":\"ok\",\"message\":false" to "api.invalid_response",
            "\"schema\":1,\"ok\":true,\"code\":\"\",\"message\":\"\"" to "api.invalid_response",
        )) {
            val content = "{$header,\"data\":{\"value\":42}}"
            val httpError = assertThrows(ModuleApiException::class.java) {
                codec.decode(200, content.byteInputStream(), serializer<Payload>())
            }
            val streamError = assertThrows(ModuleApiException::class.java) {
                codec.decode(content, serializer<Payload>())
            }
            assertEquals(code, httpError.resultCode)
            assertEquals(code, streamError.resultCode)
        }
    }

    @Test fun typedErrorsPreserveHTTPAndDomainFailurePayloads() {
        for (status in listOf(409, 500)) {
            for (persisted in listOf("true", "\"true\"", "1")) {
                val content = """{"data":{"persisted":$persisted,"revision":"fixture"},"schema":1,"ok":false,"code":"config.conflict","message":"changed"}"""
                val error = assertThrows(ModuleApiException::class.java) {
                    codec.decode(status, content.byteInputStream(), serializer<Payload>())
                }
                assertEquals("config.conflict", error.resultCode)
                assertEquals("changed", error.message)
                assertEquals(persisted == "true", error.persisted)
                assertTrue(error.data.toString().contains("revision"))
            }
        }
        val error = assertThrows(ModuleApiException::class.java) {
            codec.decode(500, apiSuccess("""{"value":42}""", "service.failure").content.byteInputStream(), serializer<Payload>())
        }
        assertEquals("service.failure", error.resultCode)
    }

    @Test fun typedHTTP200DoesNotReparseAnIncompatibleFailureDTO() {
        val content = """{"schema":1,"ok":false,"code":"config.conflict","message":"changed","data":{"persisted":true}}"""
        val error = assertThrows(ModuleApiException::class.java) {
            codec.decode(200, content.byteInputStream(), serializer<Payload>())
        }
        assertEquals("api.invalid_response", error.resultCode)
        val eventError = assertThrows(ModuleApiException::class.java) { codec.decode(content, serializer<Payload>()) }
        assertEquals("config.conflict", eventError.resultCode)
        assertTrue(eventError.persisted)
    }

    @Test fun typedCompatibleHTTP200FailureNeverSucceeds() {
        val content = """{"schema":1,"ok":false,"code":"service.failure","message":"failed","data":{"value":42}}"""
        val error = assertThrows(ModuleApiException::class.java) {
            codec.decode(200, content.byteInputStream(), serializer<Payload>())
        }
        assertEquals("service.failure", error.resultCode)
    }

    @Test fun successStartsTypedDecodingBeforeReadingTheWholeBody() {
        val bytes = apiSuccess("""{"value":42,"padding":"${"x".repeat(256 * 1024)}"}""").content.toByteArray()
        val input = object : ByteArrayInputStream(bytes) {
            override fun read(buffer: ByteArray, offset: Int, length: Int): Int = super.read(buffer, offset, minOf(length, 64))
        }
        var decodes = 0
        val original = serializer<Payload>()
        val observed = object : KSerializer<Payload> {
            override val descriptor = original.descriptor
            override fun serialize(encoder: Encoder, value: Payload) = original.serialize(encoder, value)
            override fun deserialize(decoder: Decoder): Payload {
                decodes++
                assertTrue("成功路径不得先 readBytes 或全文回放再解 DTO", input.available() > 128 * 1024)
                return original.deserialize(decoder)
            }
        }
        assertEquals(Payload(42), codec.decode(200, input, observed))
        assertEquals(1, decodes)
    }

    @OptIn(kotlinx.serialization.ExperimentalSerializationApi::class)
    @Test fun largeSuccessDoesNotAllocateAWholeBodyReplay() {
        val bean = ManagementFactory.getThreadMXBean() as? ThreadMXBean
        org.junit.Assume.assumeTrue(bean?.isThreadAllocatedMemorySupported == true)
        bean!!
        bean.isThreadAllocatedMemoryEnabled = true
        val bytes = apiSuccess("""{"value":42,"padding":"${"x".repeat(2 * 1024 * 1024)}"}""").content.toByteArray()
        val serializer = serializer<Payload>()
        val json = Json { ignoreUnknownKeys = true }
        val envelope = ModuleEnvelope.serializer(serializer)
        val direct = { json.decodeFromStream(envelope, bytes.inputStream()).data }
        val throughCodec = { codec.decode(200, bytes.inputStream(), serializer) }
        repeat(3) { direct(); throughCodec() }
        val thread = Thread.currentThread().threadId()
        fun allocation(decode: () -> Payload): Long {
            val before = bean.getThreadAllocatedBytes(thread)
            repeat(4) { assertEquals(Payload(42), decode()) }
            return (bean.getThreadAllocatedBytes(thread) - before) / 4
        }
        val baseline = allocation(direct)
        val perResponse = allocation(throughCodec)
        assertTrue("codec 不应在 typed 解码之外增加全文 readBytes 或 Replay 缓冲：$perResponse / $baseline",
            perResponse < baseline + bytes.size / 2)
    }

    @Test fun malformedTypedSuccessIsNotAnErrorPayloadFallback() {
        for (content in listOf(apiSuccess("""{"value":"wrong"}""").content,
            apiSuccess("""{"value":42}""").content + "{}")) {
            val error = assertThrows(ModuleApiException::class.java) {
                codec.decode(200, content.byteInputStream(), serializer<Payload>())
            }
            assertEquals("api.invalid_response", error.resultCode)
        }
    }

    @Test fun frameMemoizesTheSameTypeButNotOtherTypes() {
        var decodes = 0
        val original = serializer<Payload>()
        val counting = object : KSerializer<Payload> {
            override val descriptor = original.descriptor
            override fun serialize(encoder: Encoder, value: Payload) = original.serialize(encoder, value)
            override fun deserialize(decoder: Decoder): Payload {
                decodes++
                return original.deserialize(decoder)
            }
        }
        val frame = ModuleEventFrame(apiSuccess("""{"value":42}""").content, codec)
        val first = frame.decode(typeOf<Payload>(), counting)
        assertSame(first, frame.decode(typeOf<Payload>(), counting))
        assertEquals(1, decodes)
        assertEquals("42", frame.decode<kotlinx.serialization.json.JsonObject>()["value"].toString())
    }

    @Test fun frameMemoizesTypedSchemaFailures() {
        val frame = ModuleEventFrame("""{"schema":2,"ok":true,"code":"ok","message":"","data":{"value":42}}""", codec)
        val first = assertThrows(ModuleApiException::class.java) { frame.decode<Payload>() }
        val second = assertThrows(ModuleApiException::class.java) { frame.decode<Payload>() }
        assertEquals("api.unsupported_schema", first.resultCode)
        assertSame(first, second)
    }
}
