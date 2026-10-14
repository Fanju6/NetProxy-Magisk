package com.fanjv.netproxy.core.api

import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonPrimitive
import java.io.File
import java.io.InputStream

internal data class ModuleHttpResponse(val status: Int, val content: String)

internal data class ApiTestRequest(val method: String, val path: String, val body: JsonObject, val query: Map<String, String>, val mutation: Boolean) {
    val target: String get() = query.getValue("target")
    fun text(key: String) = body.getValue(key).jsonPrimitive.content
    fun assertConfig(method: String, target: String, revision: String? = null) {
        org.junit.Assert.assertEquals(method, this.method)
        org.junit.Assert.assertEquals(if (method == "POST") "/config/validate" else "/config", path)
        org.junit.Assert.assertEquals(target, this.target)
        if (revision != null) org.junit.Assert.assertEquals(revision, text("revision"))
    }
}

internal abstract class TestApiTransport : ModuleApiTransport {
    override suspend fun reconnect(address: String): Unit = error("Unexpected reconnect")
    final override suspend fun <T> request(method: String, path: String, body: JsonObject?, query: Map<String, String>, mutation: Boolean,
        decode: (Int, InputStream) -> T): T {
        val response = respond(ApiTestRequest(method, path, body ?: JsonObject(emptyMap()), query, mutation))
        return response.content.byteInputStream().use { decode(response.status, it) }
    }
    abstract suspend fun respond(request: ApiTestRequest): ModuleHttpResponse
    override suspend fun download(path: String, query: Map<String, String>, destination: File): Unit = error("Unexpected download")
    override suspend fun image(path: String): ByteArray = error("Unexpected image")
    override suspend fun watch(topics: Set<String>, consume: suspend (ModuleEvent) -> Unit): Unit = kotlinx.coroutines.awaitCancellation()
}

internal fun TestApiTransport(block: suspend (ApiTestRequest) -> ModuleHttpResponse) = object : TestApiTransport() {
    override suspend fun respond(request: ApiTestRequest) = block(request)
}

internal fun apiSuccess(data: String, code: String = "test") = ModuleHttpResponse(200,
    """{"schema":1,"ok":true,"code":${JsonPrimitive(code)},"message":"","data":$data}""")
