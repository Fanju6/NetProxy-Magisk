package com.fanjv.netproxy.core.api

import android.content.Context
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.channels.awaitClose
import kotlinx.coroutines.channels.trySendBlocking
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.cancelAndJoin
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.launch
import kotlinx.coroutines.flow.buffer
import kotlinx.coroutines.flow.callbackFlow
import kotlinx.coroutines.flow.collect
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import okhttp3.Call
import okhttp3.Callback
import okhttp3.ConnectionPool
import okhttp3.HttpUrl.Companion.toHttpUrl
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Protocol
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.Response
import java.io.File
import java.io.FilterInputStream
import java.io.IOException
import java.io.InputStream
import java.io.InterruptedIOException
import java.net.InetAddress
import java.net.InetSocketAddress
import java.net.Proxy
import java.net.ProtocolException
import java.net.Socket
import java.security.MessageDigest
import java.security.SecureRandom
import java.security.cert.CertificateException
import java.security.cert.X509Certificate
import java.util.concurrent.TimeUnit
import javax.net.ssl.SSLContext
import javax.net.ssl.SSLHandshakeException
import javax.net.ssl.SSLPeerUnverifiedException
import javax.net.ssl.SSLProtocolException
import javax.net.ssl.X509TrustManager
import javax.net.SocketFactory
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException

internal class LocalHttpTransport internal constructor(
    private val connections: ConnectionStore,
    private val defaultAddress: String = "https://127.0.0.1:9091",
    private val allowProvisioning: Boolean = true,
) : ModuleApiTransport {
    constructor(context: Context) : this(ConnectionStore(context.applicationContext))

    internal val json = Json { ignoreUnknownKeys = true; explicitNulls = false }
    private val pairing = Mutex()
    private val requestDispatcher = okhttp3.Dispatcher().apply { maxRequestsPerHost = 8 }
    private val eventDispatcher = okhttp3.Dispatcher().apply { maxRequests = 1; maxRequestsPerHost = 1 }
    @Volatile private var address = defaultAddress
    @Volatile private var pinnedClient: Pair<String, OkHttpClient>? = null
    private val provisioningClient by lazy { client(null).newBuilder().connectTimeout(3, TimeUnit.SECONDS).build() }

    suspend fun connect(): ModuleConnection = withContext(Dispatchers.IO) {
        connections.read()?.let { return@withContext it }
        pairing.withLock { connections.read() ?: provisionConnection(address) }
    }

    override suspend fun reconnect(address: String): Unit = withContext(Dispatchers.IO) {
        validateModuleAddress(address)
        pairing.withLock {
            provisionConnection(address)
            this@LocalHttpTransport.address = address
        }
    }

    private suspend fun provisionConnection(address: String): ModuleConnection {
        if (!allowProvisioning) throw ModuleApiException("api.connection_unavailable", "需要通过可信私有交付建立连接")
        fun receipt(): ModuleConnection? = try { connections.read() } catch (error: Exception) {
            if (error is CancellationException) throw error
            null
        }
        val previous = receipt()
        // 未认证连接只触发私有 Provider 交付，独立客户端不携带 Bearer，也不复用业务连接。
        val request = Request.Builder().url("$address/api/v1/auth/provision")
            .post("{}".toRequestBody(JSON_MEDIA_TYPE)).build()
        return coroutineScope {
            val trigger = launch {
                try {
                    // 响应完全不可信，不解析其正文、错误码或身份；只等待 Root Provider 的新交付。
                    execute(provisioningClient, request, null, 20_000) { }
                } catch (error: Exception) {
                    if (error is CancellationException) throw error
                }
            }
            try {
                repeat(200) { attempt ->
                    receipt()?.takeIf { it !== previous && it.address == address }?.let { return@coroutineScope it }
                    if (trigger.isCompleted && attempt >= 20) {
                        throw ModuleApiException("api.connection_unavailable", "无法自动连接模块后台")
                    }
                    delay(100)
                }
                throw ModuleApiException("api.connection_unavailable", "无法自动连接模块后台")
            } finally {
                trigger.cancelAndJoin()
            }
        }
    }

    override suspend fun <T> request(
        method: String,
        path: String,
        body: JsonObject?,
        query: Map<String, String>,
        mutation: Boolean,
        decode: (Int, InputStream) -> T,
    ): T {
        val credentials = connect()
        val timeout = when {
            path == "/android/apps" -> 60_000L
            !mutation -> 30_000L
            path.startsWith("/subscriptions") -> 615_000L
            else -> 135_000L
        }
        val request = requestBuilder(credentials, path, query)
            .method(method, body?.toString()?.toRequestBody(JSON_MEDIA_TYPE)).build()
        return execute(pinned(credentials.certificate_sha256), request, credentials, timeout) { response ->
            decode(response.code, LimitedInputStream(response.body.byteStream(), RESPONSE_LIMIT))
        }
    }

    override suspend fun image(path: String): ByteArray {
        val credentials = connect()
        return execute(pinned(credentials.certificate_sha256), requestBuilder(credentials, path, emptyMap()).build(),
            credentials, 15_000) { response ->
            if (response.code != 200) fail(response, "api.image_failed", "无法读取应用图标")
            require(response.body.contentType()?.toString() == "image/png")
            LimitedInputStream(response.body.byteStream(), 256 * 1024).readBytes()
        }
    }

    override suspend fun download(path: String, query: Map<String, String>, destination: File) {
        try {
            val credentials = connect()
            execute(pinned(credentials.certificate_sha256), requestBuilder(credentials, path, query).build(),
                credentials, 150_000) { response ->
                if (!response.isSuccessful) fail(response, "api.download_failed", "诊断包下载失败")
                require(response.body.contentType()?.toString() == "application/gzip")
                try {
                    val count = destination.outputStream().use { output -> response.body.byteStream().copyTo(output) }
                    val length = response.body.contentLength()
                    require(count > 0 && (length < 0 || count == length))
                } catch (error: Throwable) {
                    // 取消可能先唤醒调用方，正文线程须在关闭文件后再次清理部分下载。
                    destination.delete()
                    throw error
                }
            }
        } catch (error: Throwable) {
            destination.delete()
            throw error
        }
    }

    override suspend fun watch(topics: Set<String>, consume: suspend (ModuleEvent) -> Unit) {
        val credentials = connect()
        val request = requestBuilder(credentials, "/events", mapOf("topics" to topics.sorted().joinToString(",")))
            .header("Connection", "close").build()
        val client = pinned(credentials.certificate_sha256).newBuilder()
            .dispatcher(eventDispatcher)
            .connectionPool(ConnectionPool(0, 1, TimeUnit.SECONDS))
            .protocols(listOf(Protocol.HTTP_1_1)).readTimeout(30, TimeUnit.SECONDS).build()
        callbackFlow<ModuleEvent> {
            val call = client.newCall(request)
            call.enqueue(object : Callback {
                override fun onFailure(call: Call, e: IOException) {
                    close(transportError(e, credentials, call))
                }

                override fun onResponse(call: Call, response: Response) {
                    try {
                        response.use {
                            invalidateUnauthorized(response, credentials)
                            if (response.code != 200) fail(response, "api.events_failed", "无法订阅模块事件")
                            require(response.body.contentType()?.let { it.type == "text" && it.subtype == "event-stream" } == true)
                            val source = response.body.source()
                            var name = ""
                            val data = StringBuilder()
                            var size = 0L
                            while (!call.isCanceled() && !source.exhausted()) {
                                val line = source.readUtf8LineStrict(RESPONSE_LIMIT - size)
                                size += line.toByteArray(Charsets.UTF_8).size + 1
                                require(size <= RESPONSE_LIMIT)
                                when {
                                    line.isEmpty() -> {
                                        if (data.isNotEmpty()) {
                                            require(name in EVENT_NAMES)
                                            if (trySendBlocking(ModuleEvent(name, data.toString())).isFailure) return
                                        }
                                        name = ""
                                        data.setLength(0)
                                        size = 0
                                    }
                                    line.startsWith("event:") -> name = line.substringAfter(':').trim()
                                    line.startsWith("data:") -> {
                                        if (data.isNotEmpty()) data.append('\n')
                                        data.append(line.substringAfter(':').removePrefix(" "))
                                    }
                                }
                            }
                            throw ModuleApiException("api.disconnected", "模块事件连接已断开")
                        }
                    } catch (error: Exception) {
                        close(transportError(error, credentials, call))
                    }
                }
            })
            awaitClose { call.cancel() }
        }.buffer(1).collect { consume(it) }
    }

    private fun requestBuilder(credentials: ModuleConnection, path: String, query: Map<String, String>): Request.Builder {
        require(path.startsWith('/') && !path.contains('?') && !path.contains('#'))
        val url = (credentials.address + "/api/v1" + path).toHttpUrl().newBuilder()
        query.forEach { (key, value) -> url.addQueryParameter(key, value) }
        return Request.Builder().url(url.build()).header("Authorization", "Bearer ${credentials.secret}")
    }

    private suspend fun <T> execute(
        client: OkHttpClient,
        request: Request,
        credentials: ModuleConnection?,
        timeout: Long,
        decode: (Response) -> T,
    ): T = suspendCancellableCoroutine { continuation ->
        val policy = RequestPolicy(timeout.toInt(), request.method == "GET")
        val call = client.newCall(request.newBuilder().tag(RequestPolicy::class.java, policy).build())
        call.timeout().timeout(timeout + 5_000, TimeUnit.MILLISECONDS)
        // continuation 保持到正文解码结束，取消始终只作用于当前 Call。
        continuation.invokeOnCancellation { call.cancel() }
        call.enqueue(object : Callback {
            override fun onFailure(call: Call, e: IOException) {
                continuation.resumeWithException(transportError(e, credentials, call))
            }

            override fun onResponse(call: Call, response: Response) {
                try {
                    val result = response.use {
                        invalidateUnauthorized(response, credentials)
                        decode(response)
                    }
                    continuation.resume(result)
                } catch (error: Exception) {
                    continuation.resumeWithException(transportError(error, credentials, call))
                }
            }
        })
    }

    private fun invalidateUnauthorized(response: Response, credentials: ModuleConnection?) {
        if (response.code == 401 && credentials != null) {
            if (connections.invalidate(credentials)) address = credentials.address
        }
    }

    private fun transportError(error: Exception, credentials: ModuleConnection?, call: Call): Exception = when {
        error.isIdentityFailure() && !call.isCanceled() && credentials != null -> {
            if (connections.invalidate(credentials)) address = credentials.address
            ModuleApiException("api.unauthorized", "模块身份验证失败，请重新连接")
        }
        error is IOException -> ModuleApiException("api.connection_failed", "无法连接模块后台，操作结果需要重新确认")
        else -> error
    }

    private fun Throwable.isIdentityFailure(): Boolean = generateSequence(this) { it.cause }.any {
        it is SSLPeerUnverifiedException || it is CertificateException
    }

    private fun fail(response: Response, code: String, message: String): Nothing {
        ModuleApiCodec(json).decode(response.code, LimitedInputStream(response.body.byteStream(), 64 * 1024))
        throw ModuleApiException(code, message)
    }

    private fun pinned(fingerprint: String): OkHttpClient {
        pinnedClient?.takeIf { it.first == fingerprint }?.let { return it.second }
        return synchronized(this) {
            pinnedClient?.takeIf { it.first == fingerprint }?.second
                ?: client(fingerprint).also { pinnedClient = fingerprint to it }
        }
    }

    private fun client(fingerprint: String?): OkHttpClient {
        val expected = fingerprint?.chunked(2)?.map { it.toInt(16).toByte() }?.toByteArray()
        val trust = object : X509TrustManager {
            override fun getAcceptedIssuers(): Array<X509Certificate> = emptyArray()
            override fun checkClientTrusted(chain: Array<out X509Certificate>?, authType: String?) = throw CertificateException()
            override fun checkServerTrusted(chain: Array<out X509Certificate>?, authType: String?) {
                if (expected == null) return
                val certificate = chain?.singleOrNull() ?: throw CertificateException()
                val actual = MessageDigest.getInstance("SHA-256").digest(certificate.encoded)
                if (!MessageDigest.isEqual(actual, expected)) throw CertificateException()
            }
        }
        val tls = SSLContext.getInstance("TLS").apply { init(null, arrayOf(trust), SecureRandom()) }
        return OkHttpClient.Builder().proxy(Proxy.NO_PROXY).socketFactory(DirectSocketFactory)
            .dispatcher(requestDispatcher).connectionPool(ConnectionPool(5, 30, TimeUnit.SECONDS))
            .followRedirects(false).followSslRedirects(false).retryOnConnectionFailure(false)
            .addInterceptor { chain ->
                val policy = chain.request().tag(RequestPolicy::class.java)
                val timed = if (policy == null) chain else chain.withReadTimeout(policy.readTimeout, TimeUnit.MILLISECONDS)
                try {
                    timed.proceed(chain.request())
                } catch (error: IOException) {
                    // 只恢复尚未取得响应的只读请求，正文解码、超时、取消和身份失败不重试。
                    if (policy?.retryRead != true || chain.call().isCanceled() || error is InterruptedIOException ||
                        error.isIdentityFailure() || error is SSLHandshakeException ||
                        error is SSLProtocolException || error is ProtocolException) throw error
                    timed.proceed(chain.request())
                }
            }
            .addNetworkInterceptor { chain ->
                val response = chain.proceed(chain.request())
                // OkHttp 的 503 Retry-After 重发不受连接重试开关控制，领域请求必须只发一次。
                if (response.code == 503) response.newBuilder().removeHeader("Retry-After").build() else response
            }
            .connectTimeout(5, TimeUnit.SECONDS).sslSocketFactory(tls.socketFactory, trust).build()
    }

    private companion object {
        val JSON_MEDIA_TYPE = "application/json; charset=utf-8".toMediaType()
        const val RESPONSE_LIMIT = 8 * 1024 * 1024
        val EVENT_NAMES = setOf("status", "traffic", "groups", "subscriptions")
    }

    private data class RequestPolicy(val readTimeout: Int, val retryRead: Boolean)
}

private object DirectSocketFactory : SocketFactory() {
    // 同时绕过底层 Socket 的 SOCKS ProxySelector，不能只禁用 HTTP 代理。
    override fun createSocket() = Socket(Proxy.NO_PROXY)
    override fun createSocket(host: String, port: Int) = connected(InetSocketAddress(host, port))
    override fun createSocket(host: InetAddress, port: Int) = connected(InetSocketAddress(host, port))
    override fun createSocket(host: String, port: Int, local: InetAddress, localPort: Int) =
        connected(InetSocketAddress(host, port), InetSocketAddress(local, localPort))
    override fun createSocket(host: InetAddress, port: Int, local: InetAddress, localPort: Int) =
        connected(InetSocketAddress(host, port), InetSocketAddress(local, localPort))

    private fun connected(remote: InetSocketAddress, local: InetSocketAddress? = null): Socket {
        val socket = createSocket()
        try {
            if (local != null) socket.bind(local)
            socket.connect(remote)
            return socket
        } catch (error: Exception) {
            runCatching { socket.close() }
            throw error
        }
    }
}

internal class LimitedInputStream(input: InputStream, private val limit: Int) : FilterInputStream(input) {
    private var count = 0

    override fun read(): Int {
        checkLimit()
        val value = `in`.read()
        if (value >= 0) record(1)
        return value
    }

    override fun read(buffer: ByteArray, offset: Int, length: Int): Int {
        if (offset < 0 || length < 0 || offset > buffer.size - length) throw IndexOutOfBoundsException()
        if (length == 0) return 0
        checkLimit()
        val size = `in`.read(buffer, offset, minOf(length, limit - count + 1))
        if (size > 0) record(size)
        return size
    }

    override fun skip(size: Long): Long {
        if (size <= 0) return 0
        checkLimit()
        val skipped = `in`.skip(minOf(size, (limit - count + 1).toLong()))
        record(skipped.toInt())
        return skipped
    }

    override fun markSupported() = false
    override fun available(): Int = minOf(`in`.available(), (limit - count).coerceAtLeast(0))
    override fun mark(readlimit: Int) = Unit
    override fun reset(): Unit = throw IOException("Response stream cannot reset")

    private fun record(size: Int) {
        count += size
        checkLimit()
    }

    private fun checkLimit() {
        if (count > limit) throw IOException("Response body exceeds limit")
    }
}
