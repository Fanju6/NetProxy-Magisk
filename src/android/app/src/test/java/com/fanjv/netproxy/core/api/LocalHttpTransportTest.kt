package com.fanjv.netproxy.core.api

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitCancellation
import kotlinx.coroutines.cancelAndJoin
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import mockwebserver3.Dispatcher
import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer
import mockwebserver3.RecordedRequest
import mockwebserver3.SocketEffect
import okhttp3.Protocol
import org.junit.After
import org.junit.Assert.*
import org.junit.Before
import org.junit.Test
import java.io.ByteArrayInputStream
import java.io.File
import java.io.IOException
import java.net.InetAddress
import java.net.Proxy
import java.net.ProxySelector
import java.net.SocketAddress
import java.net.URI
import java.nio.file.Files
import java.security.KeyStore
import java.security.MessageDigest
import java.security.cert.CertificateException
import java.security.cert.X509Certificate
import java.util.UUID
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicInteger
import javax.net.ssl.KeyManagerFactory
import javax.net.ssl.SSLContext
import javax.net.ssl.SSLException
import javax.net.ssl.SSLHandshakeException
import javax.net.ssl.SSLPeerUnverifiedException
import javax.net.ssl.SSLProtocolException
import javax.net.ssl.SSLSocketFactory

class LocalHttpTransportTest {
    private lateinit var server: MockWebServer
    private lateinit var store: ConnectionStore
    private lateinit var transport: LocalHttpTransport
    private lateinit var credentials: ModuleConnection
    private val diskReads = AtomicInteger()

    @Before fun setUp() {
        server = MockWebServer().apply {
            protocols = listOf(Protocol.HTTP_1_1)
            useHttps(identity.socketFactory)
            start(InetAddress.getByName("127.0.0.1"), 0)
        }
        credentials = ModuleConnection("https://127.0.0.1:${server.port}", "a".repeat(64), identity.fingerprint)
        store = ConnectionStore({ diskReads.incrementAndGet(); Json.encodeToString(credentials) }, {})
        transport = LocalHttpTransport(store, credentials.address)
    }

    @After fun tearDown() { server.close() }

    @Test fun reusesPinnedConnectionAndCachedCredentials() = runBlocking {
        server.enqueue(MockResponse(body = "first"))
        server.enqueue(MockResponse(body = "second"))
        assertEquals("first", read())
        assertEquals("second", read())
        val first = recorded()
        val second = recorded()
        assertEquals(first.connectionIndex, second.connectionIndex)
        assertEquals(1, second.exchangeIndex)
        assertEquals("Bearer ${credentials.secret}", first.headers["Authorization"])
        assertEquals(1, diskReads.get())
    }

    @Test fun cancellationAfterCompletionDoesNotDisconnectReusedConnection() = runBlocking {
        server.enqueue(MockResponse(body = "first"))
        server.enqueue(MockResponse.Builder().body("second").bodyDelay(1, TimeUnit.SECONDS).build())
        val first = async(Dispatchers.IO) { read() }
        assertEquals("first", first.await())
        val firstRequest = recorded()
        val decoding = CountDownLatch(1)
        val second = async(Dispatchers.IO) {
            transport.request("GET", "/info", null, emptyMap(), false) { _, input ->
                decoding.countDown()
                input.readBytes().toString(Charsets.UTF_8)
            }
        }
        assertTrue(decoding.await(3, TimeUnit.SECONDS))
        first.cancel()
        assertEquals("second", withTimeout(3_000) { second.await() })
        assertEquals(firstRequest.connectionIndex, recorded().connectionIndex)
    }

    @Test fun cancellationWhileDecodingBodyCancelsOnlyItsCall() = runBlocking {
        server.enqueue(MockResponse.Builder().body("delayed").bodyDelay(1, TimeUnit.SECONDS).build())
        server.enqueue(MockResponse(body = "healthy"))
        val decoding = CountDownLatch(1)
        val released = CountDownLatch(1)
        val job = launch(Dispatchers.IO) {
            transport.request("GET", "/info", null, emptyMap(), false) { _, input ->
                decoding.countDown()
                try { input.readBytes() } finally { released.countDown() }
            }
        }
        assertTrue(decoding.await(3, TimeUnit.SECONDS))
        job.cancelAndJoin()
        assertTrue(released.await(1, TimeUnit.SECONDS))
        assertEquals("healthy", read())
        assertEquals(2, server.requestCount)
        assertNotNull(store.read())
    }

    @Test fun decoderFailureClosesResponseAndRetainsCredentials() = runBlocking {
        server.enqueue(MockResponse(body = "first"))
        server.enqueue(MockResponse(body = "second"))
        val failure = IllegalArgumentException("test decoder")
        val result = runCatching {
            transport.request("GET", "/info", null, emptyMap(), false) { _, _ -> throw failure }
        }
        assertTrue(result.exceptionOrNull() is IllegalArgumentException)
        assertEquals(failure.message, result.exceptionOrNull()?.message)
        assertEquals("second", read())
        assertNotNull(store.read())
    }

    @Test fun staleUnauthorizedResponseDoesNotClearProviderReplacement() = runBlocking {
        server.enqueue(MockResponse.Builder().code(401).body("denied").headersDelay(300, TimeUnit.MILLISECONDS).build())
        server.enqueue(MockResponse(body = "new credentials"))
        val old = async(Dispatchers.IO) { readStatus() }
        recorded()
        val replacement = credentials.copy(secret = "c".repeat(64))
        store.write(Json.encodeToString(replacement))
        assertEquals(401, old.await())
        assertEquals(replacement, store.read())
        assertEquals("new credentials", read())
        assertEquals("Bearer ${replacement.secret}", recorded().headers["Authorization"])
    }

    @Test fun unauthorizedMutationInvalidatesWithoutResending() = runBlocking {
        server.enqueue(MockResponse(code = 401, body = "denied"))
        val status = transport.request("POST", "/service/start", JsonObject(emptyMap()), emptyMap(), true) { code, input ->
            input.readBytes()
            code
        }
        assertEquals(401, status)
        assertNull(store.read())
        assertEquals(1, server.requestCount)
        recorded()
        assertNull(server.takeRequest(100, TimeUnit.MILLISECONDS))
    }

    @Test fun reconnectPreservesOldCredentialsUntilFreshDeliveryAndNeverSendsBearer() = runBlocking {
        val old = requireNotNull(store.read())
        val delivered = credentials.copy(secret = "c".repeat(64))
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                assertEquals("/api/v1/auth/provision", request.target)
                assertEquals("POST", request.method)
                assertNull(request.headers["Authorization"])
                assertSame(old, store.read())
                store.write(Json.encodeToString(delivered))
                return MockResponse(body = "{}")
            }
        }
        transport.reconnect(credentials.address)
        assertEquals(delivered, transport.connect())
        assertFalse(store.invalidate(old))
        assertEquals(1, server.requestCount)
        assertEquals(1, diskReads.get())
    }

    @Test fun failedReconnectKeepsCredentialsAndAutomaticConnectionAddress() = runBlocking {
        val old = requireNotNull(store.read())
        MockWebServer().use { unavailable ->
            unavailable.useHttps(identity.socketFactory)
            unavailable.start(InetAddress.getByName("127.0.0.1"), 0)
            unavailable.enqueue(MockResponse(code = 503,
                body = """{"schema":1,"ok":false,"code":"api.busy","message":"busy","data":{}}"""))
            val error = runCatching { transport.reconnect("https://127.0.0.1:${unavailable.port}") }.exceptionOrNull() as ModuleApiException
            assertEquals("api.connection_unavailable", error.resultCode)
            assertSame(old, store.read())
        }
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                if (request.target == "/api/v1/auth/provision") {
                    store.write(Json.encodeToString(credentials))
                    return MockResponse(body = "{}")
                }
                return MockResponse(body = "retained")
            }
        }
        assertEquals("retained", read())
        assertTrue(store.invalidate(old))
        assertEquals(credentials, transport.connect())
        assertNotSame(old, store.read())
    }

    @Test fun reconnectDoesNotMistakeOldSameAddressCredentialsForNewDelivery() = runBlocking {
        val old = requireNotNull(store.read())
        server.enqueue(MockResponse(body = "{}"))
        val error = runCatching { transport.reconnect(credentials.address) }.exceptionOrNull() as ModuleApiException
        assertEquals("api.connection_unavailable", error.resultCode)
        assertSame(old, store.read())
        assertEquals(1, server.requestCount)
    }

    @Test fun explicitReconnectRepairsUnreadableLocalReceiptOnlyThroughProvider() = runBlocking {
        val broken = ConnectionStore({ "{broken" }, {})
        val reconnecting = LocalHttpTransport(broken, credentials.address)
        assertTrue(runCatching { broken.read() }.isFailure)
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                broken.write(Json.encodeToString(credentials))
                return MockResponse(body = "{}")
            }
        }
        reconnecting.reconnect(credentials.address)
        assertEquals(credentials, broken.read())
        assertEquals(1, server.requestCount)
    }

    @Test fun cancelledReconnectDoesNotDiscardWorkingCredentials() = runBlocking {
        val old = requireNotNull(store.read())
        server.enqueue(MockResponse.Builder().body("{}").headersDelay(1, TimeUnit.SECONDS).build())
        val pairing = launch(Dispatchers.IO) { transport.reconnect(credentials.address) }
        recorded()
        pairing.cancelAndJoin()
        assertSame(old, store.read())
        server.enqueue(MockResponse(body = "still connected"))
        assertEquals("still connected", read())
    }

    @Test fun deliverySucceedsIndependentlyOfUntrustedResponseStatus() = runBlocking {
        val old = requireNotNull(store.read())
        val replacement = credentials.copy(secret = "c".repeat(64))
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                store.write(Json.encodeToString(replacement))
                return MockResponse(code = 503,
                    body = """{"schema":1,"ok":false,"code":"api.busy","message":"busy","data":{}}""")
            }
        }
        transport.reconnect(credentials.address)
        assertEquals(replacement, store.read())
        assertFalse(store.invalidate(old))
    }

    @Test fun forgedBootstrapCredentialsNeverBecomeTrusted() = runBlocking {
        val empty = ConnectionStore({ null }, {})
        val connecting = LocalHttpTransport(empty, credentials.address)
        server.enqueue(MockResponse(body = Json.encodeToString(credentials)))
        val error = runCatching { connecting.connect() }.exceptionOrNull() as ModuleApiException
        assertEquals("api.connection_unavailable", error.resultCode)
        assertNull(empty.read())
        val request = recorded()
        assertEquals("POST", request.method)
        assertEquals("/api/v1/auth/provision", request.target)
        assertEquals("{}", request.body?.utf8())
        assertNull(request.headers["Authorization"])
        assertNull(request.headers["Cookie"])
        assertEquals(1, server.requestCount)
    }

    @Test fun freshProviderDeliveryCancelsSlowBootstrapBeforeAuthenticatedRead() = runBlocking {
        val empty = ConnectionStore({ null }, {})
        val connecting = LocalHttpTransport(empty, credentials.address)
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                if (request.target == "/api/v1/auth/provision") {
                    assertNull(request.headers["Authorization"])
                    empty.write(Json.encodeToString(credentials))
                    return MockResponse.Builder().headersDelay(3, TimeUnit.SECONDS).body("not trusted").build()
                }
                assertEquals("Bearer ${credentials.secret}", request.headers["Authorization"])
                return MockResponse(body = "trusted")
            }
        }
        assertEquals("trusted", withTimeout(2_000) {
            connecting.request("GET", "/info", null, emptyMap(), false) { _, input -> input.readBytes().decodeToString() }
        })
        assertEquals(2, server.requestCount)
    }

    @Test fun concurrentFirstReadsShareOnePrivateDelivery() = runBlocking {
        val empty = ConnectionStore({ null }, {})
        val connecting = LocalHttpTransport(empty, credentials.address)
        val triggers = AtomicInteger()
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                if (request.target == "/api/v1/auth/provision") {
                    triggers.incrementAndGet()
                    empty.write(Json.encodeToString(credentials))
                }
                return MockResponse(body = "trusted")
            }
        }
        val calls = List(8) { async(Dispatchers.IO) { connecting.connect() } }
        calls.forEach { assertEquals(credentials, it.await()) }
        assertEquals(1, triggers.get())
    }

    @Test fun actualUnauthorizedReadRequiresFreshProviderDeliveryWithoutReplayingRead() = runBlocking {
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                if (request.target == "/api/v1/auth/provision") {
                    assertNull(request.headers["Authorization"])
                    store.write(Json.encodeToString(credentials.copy(secret = "c".repeat(64))))
                    return MockResponse(body = "ignored")
                }
                return if (request.headers["Authorization"] == "Bearer ${credentials.secret}")
                    MockResponse(code = 401, body = "denied") else MockResponse(body = "recovered")
            }
        }
        assertEquals(401, readStatus())
        assertNull(store.read())
        assertEquals(1, server.requestCount)
        assertEquals("recovered", read())
        assertEquals(3, server.requestCount)
    }

    @Test fun cancellationBeforeResponseHeadersDoesNotRetryRead() = runBlocking {
        server.enqueue(MockResponse.Builder().body("late").headersDelay(1, TimeUnit.SECONDS).build())
        val reading = launch(Dispatchers.IO) { read() }
        recorded()
        reading.cancelAndJoin()
        assertNull(server.takeRequest(200, TimeUnit.MILLISECONDS))
        assertEquals(1, server.requestCount)
        assertNotNull(store.read())
    }

    @Test fun stalePooledConnectionRecoversReadWithoutLosingCredentials() = runBlocking {
        server.enqueue(MockResponse.Builder().body("first").onResponseEnd(SocketEffect.CloseSocket()).build())
        assertEquals("first", read())
        delay(200)
        server.enqueue(MockResponse(body = "recovered"))
        assertEquals("recovered", read())
        assertEquals(2, server.requestCount)
        assertNotNull(store.read())
    }

    @Test fun tlsConnectionResetRecoversReadOnceWithoutInvalidatingCredentials() = runBlocking {
        val old = requireNotNull(store.read())
        val attempts = AtomicInteger()
        failBeforeResponse {
            if (attempts.incrementAndGet() == 1) SSLException("Read error: Connection reset by peer") else null
        }
        server.enqueue(MockResponse(body = "recovered"))
        assertEquals("recovered", read())
        assertEquals(2, attempts.get())
        assertSame(old, store.read())
        assertEquals(1, server.requestCount)
    }

    @Test fun repeatedTlsConnectionResetDoesNotDiscardCredentialsOrReplayWrites() = runBlocking {
        val old = requireNotNull(store.read())
        val attempts = AtomicInteger()
        failBeforeResponse {
            attempts.incrementAndGet()
            SSLException("Read error: Connection reset by peer")
        }
        assertEquals("api.connection_failed", (runCatching { read() }.exceptionOrNull() as ModuleApiException).resultCode)
        assertEquals(2, attempts.get())
        for (method in listOf("POST", "PUT", "DELETE")) {
            val before = attempts.get()
            val error = runCatching {
                transport.request(method, "/service/start", JsonObject(emptyMap()), emptyMap(), true) { _, input -> input.readBytes() }
            }.exceptionOrNull() as ModuleApiException
            assertEquals("api.connection_failed", error.resultCode)
            assertEquals(before + 1, attempts.get())
            assertSame(old, store.read())
        }
    }

    @Test fun certificateAndPeerFailuresNeverRetryEvenWhenWrapped() = runBlocking {
        val failures = listOf(
            SSLPeerUnverifiedException("Hostname mismatch"),
            SSLHandshakeException("Certificate rejected").apply { initCause(CertificateException()) },
            SSLException("Wrapped certificate failure", IOException(CertificateException())),
            IOException(SSLPeerUnverifiedException("Wrapped hostname mismatch")),
        )
        for (failure in failures) {
            store.write(Json.encodeToString(credentials))
            transport = LocalHttpTransport(store, credentials.address)
            val attempts = AtomicInteger()
            failBeforeResponse { attempts.incrementAndGet(); failure }
            val error = runCatching { read() }.exceptionOrNull() as ModuleApiException
            assertEquals("api.unauthorized", error.resultCode)
            assertEquals(1, attempts.get())
            assertNull(store.read())
        }
    }

    @Test fun nonIdentityHandshakeAndProtocolFailuresRetainCredentialsWithoutRetry() = runBlocking {
        val old = requireNotNull(store.read())
        for (failure in listOf(SSLHandshakeException("Handshake aborted"), SSLProtocolException("Invalid TLS record"))) {
            transport = LocalHttpTransport(store, credentials.address)
            val attempts = AtomicInteger()
            failBeforeResponse { attempts.incrementAndGet(); failure }
            val error = runCatching { read() }.exceptionOrNull() as ModuleApiException
            assertEquals("api.connection_failed", error.resultCode)
            assertEquals(1, attempts.get())
            assertSame(old, store.read())
        }
    }

    @Test fun tlsBodyFailureDoesNotRetryOrInvalidateCredentials() = runBlocking {
        val old = requireNotNull(store.read())
        server.enqueue(MockResponse(body = "response started"))
        val error = runCatching {
            transport.request("GET", "/info", null, emptyMap(), false) { _, _ -> throw SSLException("Read reset") }
        }.exceptionOrNull() as ModuleApiException
        assertEquals("api.connection_failed", error.resultCode)
        assertEquals(1, server.requestCount)
        assertSame(old, store.read())
    }

    @Test fun tlsEventFailureKeepsCredentialsWithoutRetryingTheStream() = runBlocking {
        val old = requireNotNull(store.read())
        val attempts = AtomicInteger()
        failBeforeResponse { attempts.incrementAndGet(); SSLException("Read reset") }
        val error = runCatching { transport.watch(setOf("status")) {} }.exceptionOrNull() as ModuleApiException
        assertEquals("api.connection_failed", error.resultCode)
        assertEquals(1, attempts.get())
        assertSame(old, store.read())
    }

    @Test fun headerFailureRetriesReadOnlyOnceAndNeverRepeatsWrites() = runBlocking {
        repeat(2) { server.enqueue(MockResponse.Builder().onResponseStart(SocketEffect.CloseSocket()).build()) }
        assertEquals("api.connection_failed", (runCatching { read() }.exceptionOrNull() as ModuleApiException).resultCode)
        assertEquals(2, server.requestCount)
        for (method in listOf("POST", "PUT", "DELETE")) {
            server.enqueue(MockResponse.Builder().onResponseStart(SocketEffect.CloseSocket()).build())
            val before = server.requestCount
            val result = runCatching {
                transport.request(method, "/service/start", JsonObject(emptyMap()), emptyMap(), true) { _, input -> input.readBytes() }
            }
            assertEquals("api.connection_failed", (result.exceptionOrNull() as ModuleApiException).resultCode)
            assertEquals(before + 1, server.requestCount)
        }
    }

    @Test fun bodyFailureIsNotRetriedAfterResponseStarted() = runBlocking {
        server.enqueue(MockResponse.Builder().body("short").setHeader("Content-Length", "100")
            .onResponseEnd(SocketEffect.CloseSocket()).build())
        assertEquals("api.connection_failed", (runCatching { read() }.exceptionOrNull() as ModuleApiException).resultCode)
        assertEquals(1, server.requestCount)
        assertNotNull(store.read())
    }

    @Test fun sseAndIconReadsLeaveControlCapacityAndUseIndependentDispatchers() = runBlocking {
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse =
                if (request.target.startsWith("/api/v1/events")) {
                    MockResponse.Builder().addHeader("Content-Type", "text/event-stream")
                        .body("event: status\ndata: {}\n\n" + ": heartbeat\n\n".repeat(100))
                        .throttleBody(24, 100, TimeUnit.MILLISECONDS).build()
                } else MockResponse(body = "control")
        }
        val subscribed = CountDownLatch(1)
        val entered = java.util.concurrent.Semaphore(0)
        val release = CountDownLatch(1)
        val watching = launch(Dispatchers.IO) {
            transport.watch(setOf("status")) { subscribed.countDown(); awaitCancellation() }
        }
        val reads = mutableListOf<kotlinx.coroutines.Deferred<Unit>>()
        fun holdReads() {
            repeat(4) {
                reads += async(Dispatchers.IO) {
                    transport.request("GET", "/android/icons/fixture", null, emptyMap(), false) { _, input ->
                        input.readBytes()
                        entered.release()
                        check(release.await(5, TimeUnit.SECONDS))
                    }
                    Unit
                }
            }
        }
        try {
            assertTrue(subscribed.await(3, TimeUnit.SECONDS))
            holdReads()
            assertTrue(entered.tryAcquire(4, 3, TimeUnit.SECONDS))
            val control = withTimeout(1_000) {
                transport.request("POST", "/service/start", JsonObject(emptyMap()), emptyMap(), true) { _, input ->
                    input.readBytes().toString(Charsets.UTF_8)
                }
            }
            assertEquals("control", control)
            holdReads()
            assertTrue("SSE 不应占用普通请求的八个名额", entered.tryAcquire(4, 3, TimeUnit.SECONDS))
        } finally {
            release.countDown()
            reads.forEach { it.await() }
            watching.cancelAndJoin()
        }
    }

    @Test fun cachedRequestsDoNotWaitForPairingLock() = runBlocking {
        val delivered = CountDownLatch(1)
        val finishProvision = CountDownLatch(1)
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                if (request.target == "/api/v1/auth/provision") {
                    store.write(Json.encodeToString(credentials))
                    delivered.countDown()
                    check(finishProvision.await(3, TimeUnit.SECONDS))
                    return MockResponse(body = "{}")
                }
                return MockResponse(body = "cached")
            }
        }
        val reconnect = async(Dispatchers.IO) { transport.reconnect(credentials.address) }
        try {
            assertTrue(delivered.await(3, TimeUnit.SECONDS))
            assertEquals("cached", withTimeout(1_000) { read() })
        } finally {
            finishProvision.countDown()
            reconnect.await()
        }
    }

    @Test fun acceptsSelfSignedPinnedCertificateButRejectsDifferentIdentity() = runBlocking {
        store.write(Json.encodeToString(credentials.copy(certificate_sha256 = "0".repeat(64))))
        server.enqueue(MockResponse(body = "must not arrive"))
        val error = runCatching { read() }.exceptionOrNull() as ModuleApiException
        assertEquals("api.unauthorized", error.resultCode)
        assertNull(store.read())
        assertEquals(0, server.requestCount)
    }

    @Test fun matchingFingerprintDoesNotBypassHostnameValidation() = runBlocking {
        server.useHttps(wrongHostIdentity.socketFactory)
        store.write(Json.encodeToString(credentials.copy(certificate_sha256 = wrongHostIdentity.fingerprint)))
        server.enqueue(MockResponse(body = "must not arrive"))
        val error = runCatching { read() }.exceptionOrNull() as ModuleApiException
        assertEquals("api.unauthorized", error.resultCode)
        assertNull(store.read())
        assertEquals(0, server.requestCount)
    }

    @Test fun pinnedInstallationIdentityDoesNotDependOnCertificateDates() = runBlocking {
        for (startDate in listOf("2000/01/01", "2050/01/01")) {
            val dated = identity("ip:127.0.0.1", startDate)
            server.useHttps(dated.socketFactory)
            store.write(Json.encodeToString(credentials.copy(certificate_sha256 = dated.fingerprint)))
            server.enqueue(MockResponse(body = "pinned identity"))
            assertEquals("pinned identity", read())
            assertNotNull(store.read())
        }
        assertEquals(2, server.requestCount)
    }

    @Test fun redirectsAreReturnedWithoutFollowingOrLeakingBearer() = runBlocking {
        MockWebServer().use { target ->
            target.start()
            server.enqueue(MockResponse.Builder().code(302).addHeader("Location", target.url("/leak")).build())
            assertEquals(302, readStatus())
            assertEquals(1, server.requestCount)
            assertEquals(0, target.requestCount)
        }
    }

    @Test fun ignoresSystemProxySelector() = runBlocking {
        val original = ProxySelector.getDefault()
        ProxySelector.setDefault(object : ProxySelector() {
            override fun select(uri: URI): List<Proxy> = error("must not select system proxy")
            override fun connectFailed(uri: URI, address: SocketAddress, error: IOException) = Unit
        })
        try {
            server.enqueue(MockResponse(body = "direct"))
            assertEquals("direct", read())
        } finally { ProxySelector.setDefault(original) }
    }

    @Test fun serviceUnavailableWithImmediateRetryDoesNotResendMutation() = runBlocking {
        server.enqueue(MockResponse.Builder().code(503).addHeader("Retry-After", "0").body("unavailable").build())
        server.enqueue(MockResponse(body = "next request"))
        val status = transport.request("POST", "/service/start", JsonObject(emptyMap()), emptyMap(), true) { code, input ->
            input.readBytes()
            code
        }
        assertEquals(503, status)
        assertEquals(1, server.requestCount)
        assertEquals("next request", read())
    }

    @Test fun queryUsesUrlEncodingWithoutChangingPath() = runBlocking {
        server.enqueue(MockResponse(body = "ok"))
        transport.request("GET", "/info", null, mapOf("value" to "a b,&?#"), false) { _, input -> input.readBytes() }
        val request = recorded()
        assertEquals("/api/v1/info", request.url.encodedPath)
        assertEquals("a b,&?#", request.url.queryParameter("value"))
    }

    @Test fun responseAndImageLimitsAreEnforced() = runBlocking {
        server.enqueue(MockResponse(body = "x".repeat(8 * 1024 * 1024 + 1)))
        assertEquals("api.connection_failed", (runCatching { read() }.exceptionOrNull() as ModuleApiException).resultCode)
        server.enqueue(MockResponse.Builder().addHeader("Content-Type", "image/png").body("x".repeat(256 * 1024)).build())
        assertEquals(256 * 1024, transport.image("/android/icons/test").size)
        server.enqueue(MockResponse.Builder().addHeader("Content-Type", "image/png").body("x".repeat(256 * 1024 + 1)).build())
        assertEquals("api.connection_failed", (runCatching { transport.image("/android/icons/test") }.exceptionOrNull() as ModuleApiException).resultCode)
    }

    @Test fun downloadWritesBinaryBodyAndCancellationRemovesPartialFile() = runBlocking {
        val parent = File("build/tmp").apply { mkdirs() }.toPath()
        val destination = Files.createTempFile(parent, "transport-download-", ".gz").toFile()
        try {
            server.enqueue(MockResponse.Builder().addHeader("Content-Type", "application/gzip").body("archive").build())
            transport.download("/logs/export", emptyMap(), destination)
            assertEquals("archive", destination.readText())
            server.enqueue(MockResponse.Builder().addHeader("Content-Type", "application/gzip").body("x".repeat(100))
                .throttleBody(1, 100, TimeUnit.MILLISECONDS).build())
            val downloading = launch(Dispatchers.IO) { transport.download("/logs/export", emptyMap(), destination) }
            withTimeout(3_000) {
                while (destination.length() != 1L) delay(10)
            }
            downloading.cancelAndJoin()
            withTimeout(2_000) {
                while (destination.exists()) delay(10)
            }
            assertFalse(destination.exists())
            assertNotNull(store.read())
        } finally { destination.delete() }
    }

    @Test fun sseCancellationDoesNotInterruptPooledShortRequests() = runBlocking {
        server.enqueue(MockResponse(body = "before"))
        assertEquals("before", read())
        val before = recorded()
        server.enqueue(MockResponse.Builder().addHeader("Content-Type", "text/event-stream")
            .body("event: status\ndata: {}\n\n" + ": heartbeat\n\n".repeat(10))
            .throttleBody(24, 100, TimeUnit.MILLISECONDS).build())
        server.enqueue(MockResponse(body = "after"))
        val consumed = CountDownLatch(1)
        val watching = launch(Dispatchers.IO) {
            transport.watch(setOf("status")) {
                assertEquals(ModuleEvent("status", "{}"), it)
                consumed.countDown()
                awaitCancellation()
            }
        }
        assertTrue(consumed.await(3, TimeUnit.SECONDS))
        val events = recorded()
        watching.cancelAndJoin()
        assertEquals("after", read())
        val after = recorded()
        assertEquals("close", events.headers["Connection"])
        assertNotEquals(before.connectionIndex, events.connectionIndex)
        assertEquals(before.connectionIndex, after.connectionIndex)
    }

    @Test fun sseEndOfStreamIsAnExplicitFailure() = runBlocking {
        server.enqueue(MockResponse.Builder().addHeader("Content-Type", "text/event-stream").body(": heartbeat\n\n").build())
        val error = runCatching { transport.watch(setOf("status")) {} }.exceptionOrNull() as ModuleApiException
        assertEquals("api.disconnected", error.resultCode)
    }

    @Test fun limitedStreamBoundsSingleBulkAndSkipReads() {
        val exact = LimitedInputStream(ByteArrayInputStream(ByteArray(4)), 4)
        assertEquals(4, exact.readBytes().size)
        val bulk = LimitedInputStream(ByteArrayInputStream(ByteArray(5)), 4)
        assertThrows(IOException::class.java) { bulk.readBytes() }
        val single = LimitedInputStream(ByteArrayInputStream(ByteArray(5)), 4)
        repeat(4) { assertEquals(0, single.read()) }
        assertThrows(IOException::class.java) { single.read() }
        val skip = LimitedInputStream(ByteArrayInputStream(ByteArray(5)), 4)
        assertThrows(IOException::class.java) { skip.skip(5) }
        assertFalse(exact.markSupported())
        assertThrows(IOException::class.java) { exact.reset() }
    }

    private suspend fun read(): String = transport.request("GET", "/info", null, emptyMap(), false) { _, input ->
        input.readBytes().toString(Charsets.UTF_8)
    }

    private suspend fun readStatus(): Int = transport.request("GET", "/info", null, emptyMap(), false) { status, input ->
        input.readBytes()
        status
    }

    private fun recorded(): RecordedRequest = requireNotNull(server.takeRequest(3, TimeUnit.SECONDS))

    private fun failBeforeResponse(failure: () -> IOException?) {
        // JVM TLS 不产生 Conscrypt 的全部异常类型，在真实传输链中注入相同类型，不给生产构造器增加测试钩子。
        val pinned = LocalHttpTransport::class.java.getDeclaredMethod("pinned", String::class.java).apply { isAccessible = true }
        val client = pinned.invoke(transport, credentials.certificate_sha256) as okhttp3.OkHttpClient
        val failing = client.newBuilder().addNetworkInterceptor { chain ->
            failure()?.let { throw it }
            chain.proceed(chain.request())
        }.build()
        LocalHttpTransport::class.java.getDeclaredField("pinnedClient").apply { isAccessible = true }
            .set(transport, credentials.certificate_sha256 to failing)
    }

    private data class Identity(val socketFactory: SSLSocketFactory, val fingerprint: String)

    private companion object {
        val identity by lazy { identity("ip:127.0.0.1,dns:localhost") }
        val wrongHostIdentity by lazy { identity("ip:127.0.0.2") }

        fun identity(san: String, startDate: String? = null): Identity {
            // 仅在测试时生成匿名临时证书，不提交任何私钥，也不增加 TLS 测试依赖。
            val parent = File("build/tmp").apply { mkdirs() }.toPath()
            val directory = Files.createTempDirectory(parent, "transport-tls-").toFile()
            val file = File(directory, "identity.p12")
            val password = UUID.randomUUID().toString().toCharArray()
            val keytool = File(System.getProperty("java.home"), "bin/keytool${if (System.getProperty("os.name").orEmpty().startsWith("Windows")) ".exe" else ""}")
            try {
                val arguments = mutableListOf(keytool.absolutePath, "-genkeypair", "-alias", "test", "-keyalg", "EC",
                    "-groupname", "secp256r1", "-dname", "CN=localhost", "-ext", "SAN=$san", "-validity", "2",
                    "-storetype", "PKCS12", "-keystore", file.absolutePath, "-storepass", String(password), "-noprompt")
                if (startDate != null) arguments += listOf("-startdate", startDate)
                val process = ProcessBuilder(arguments)
                    .redirectErrorStream(true).start()
                try {
                    check(process.waitFor(15, TimeUnit.SECONDS) && process.exitValue() == 0) { "Unable to generate test TLS identity" }
                } finally { if (process.isAlive) process.destroyForcibly() }
                val keys = KeyStore.getInstance("PKCS12").apply { file.inputStream().use { load(it, password) } }
                val managers = KeyManagerFactory.getInstance(KeyManagerFactory.getDefaultAlgorithm()).apply { init(keys, password) }
                val tls = SSLContext.getInstance("TLS").apply { init(managers.keyManagers, null, null) }
                val certificate = keys.getCertificate("test") as X509Certificate
                val fingerprint = MessageDigest.getInstance("SHA-256").digest(certificate.encoded).joinToString("") { "%02x".format(it) }
                return Identity(tls.socketFactory, fingerprint)
            } finally {
                password.fill('\u0000')
                file.delete()
                directory.delete()
            }
        }
    }
}
