package com.fanjv.netproxy.feature.kernel.presentation

import com.fanjv.netproxy.core.command.CommandFileStore
import com.fanjv.netproxy.core.command.NetProxyCtlClient
import com.fanjv.netproxy.core.command.NetProxyCtlOutput
import com.fanjv.netproxy.core.command.NetProxyCtlTransport
import com.fanjv.netproxy.core.module.ServiceRepository
import com.fanjv.netproxy.feature.inbound.data.InboundRepository
import com.fanjv.netproxy.feature.settings.data.ConfigRepository
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.NonCancellable
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.joinAll
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeout
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.util.concurrent.atomic.AtomicInteger

class SingBoxConfigViewModelTest {
    @get:Rule val folder = TemporaryFolder()

    private fun model(scope: CoroutineScope, transport: NetProxyCtlTransport): SingBoxConfigViewModel {
        val client = NetProxyCtlClient(transport = transport)
        val repository = ConfigRepository(client, CommandFileStore(folder.root))
        val service = ServiceRepository(client)
        return SingBoxConfigViewModel(repository, service, InboundRepository(repository, service), scope)
    }

    private fun output(data: JsonElement) = NetProxyCtlOutput(true,
        listOf("""{"schema":1,"ok":true,"code":"config.test","message":"","data":$data}"""), emptyList())

    private fun snapshot(revision: String) = output(JsonObject(mapOf(
        "content" to JsonPrimitive("""{"revision":"$revision"}"""), "revision" to JsonPrimitive(revision),
    )))

    private suspend fun SingBoxConfigViewModel.loaded(revision: String) = withTimeout(5_000) {
        state.first { !it.isLoadingDocument && it.activeDocumentRevision == revision }
    }

    private suspend fun CoroutineScope.finishOperations() {
        val jobs = coroutineContext[Job]!!.children.toList()
        withTimeout(5_000) { jobs.joinAll() }
    }

    @Test fun reopeningSameDocumentRejectsLateReadSuccessAndFailure() = runBlocking {
        for (fails in listOf(false, true)) {
            val entered = CompletableDeferred<Unit>()
            val release = CompletableDeferred<Unit>()
            var reads = 0
            val vm = model(this, NetProxyCtlTransport { _, _ ->
                if (++reads == 1) withContext(NonCancellable) {
                    entered.complete(Unit)
                    release.await()
                    if (fails) error("Old read failed")
                    snapshot("old")
                } else snapshot("new")
            })
            vm.openDocument("singbox/dns")
            withTimeout(5_000) { entered.await() }
            vm.openDocument("singbox/dns")
            vm.loaded("new")
            release.complete(Unit)
            finishOperations()
            assertEquals("new", vm.state.value.activeDocumentRevision)
            assertFalse(vm.state.value.documentLoadError)
            assertFalse(vm.state.value.isLoadingDocument)
        }
    }

    @Test fun latestDocumentListOwnsBothDataAndErrorState() = runBlocking {
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        var calls = 0
        val vm = model(this, NetProxyCtlTransport { _, _ ->
            if (++calls == 1) withContext(NonCancellable) {
                entered.complete(Unit)
                release.await()
                error("Old list failed")
            } else output(singBoxSchemaJson.parseToJsonElement(
                """[{"id":"singbox/dns","filename":"dns","category":"config","editable":true}]"""
            ))
        })
        vm.refreshDocuments()
        withTimeout(5_000) { entered.await() }
        vm.refreshDocuments()
        withTimeout(5_000) { vm.state.first { it.documents.isNotEmpty() && !it.isLoadingDocuments } }
        release.complete(Unit)
        finishOperations()
        assertEquals("singbox/dns", vm.state.value.documents.single().id)
        assertFalse(vm.state.value.documentsError)
    }

    @Test fun savedRevisionCannotBeOverwrittenByInFlightRead() = runBlocking {
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        var reads = 0
        val vm = model(this, NetProxyCtlTransport { args, _ ->
            when (args[1]) {
                "read" -> if (++reads == 1) snapshot("base") else withContext(NonCancellable) {
                    entered.complete(Unit)
                    release.await()
                    snapshot("old")
                }
                "apply" -> output(JsonObject(mapOf("revision" to JsonPrimitive("saved"))))
                else -> error("Unexpected command")
            }
        })
        vm.openDocument("singbox/dns")
        vm.loaded("base")
        vm.openDocument("singbox/dns")
        withTimeout(5_000) { entered.await() }
        val saved = CompletableDeferred<SingBoxDocumentSaveResult>()
        vm.saveDocument("singbox/dns", "{}", "base", onComplete = { saved.complete(it) })
        assertTrue(withTimeout(5_000) { saved.await() }.success)
        release.complete(Unit)
        finishOperations()
        assertEquals("saved", vm.state.value.activeDocumentRevision)
        assertEquals("{}", vm.state.value.activeDocumentContent)
        assertFalse(vm.state.value.documentLoadError)
    }

    @Test fun lateSaveCannotOverwriteNewOpenOrNotifyOldEditor() = runBlocking {
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        var reads = 0
        var callbacks = 0
        val vm = model(this, NetProxyCtlTransport { args, _ ->
            when (args[1]) {
                "read" -> snapshot(if (++reads == 1) "base" else "new")
                "apply" -> {
                    entered.complete(Unit)
                    release.await()
                    output(JsonObject(mapOf("revision" to JsonPrimitive("old-save"))))
                }
                else -> error("Unexpected command")
            }
        })
        vm.openDocument("singbox/dns")
        vm.loaded("base")
        vm.saveDocument("singbox/dns", "{}", "base", onComplete = { callbacks++ })
        withTimeout(5_000) { entered.await() }
        vm.openDocument("singbox/dns")
        vm.loaded("new")
        release.complete(Unit)
        finishOperations()
        assertEquals("new", vm.state.value.activeDocumentRevision)
        assertEquals(0, callbacks)
    }

    @Test fun savesRemainSerializedAndOnlyLatestCompletionIsPublished() = runBlocking {
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        val writes = AtomicInteger()
        val activeWrites = AtomicInteger()
        val maxActiveWrites = AtomicInteger()
        var oldCallbacks = 0
        val vm = model(this, NetProxyCtlTransport { args, _ ->
            when (args[1]) {
                "read" -> snapshot("base")
                "apply" -> {
                    val write = writes.incrementAndGet()
                    maxActiveWrites.accumulateAndGet(activeWrites.incrementAndGet(), ::maxOf)
                    try {
                        if (write == 1) { entered.complete(Unit); release.await() }
                        output(JsonObject(mapOf("revision" to JsonPrimitive("save-$write"))))
                    } finally {
                        activeWrites.decrementAndGet()
                    }
                }
                else -> error("Unexpected command")
            }
        })
        vm.openDocument("singbox/dns")
        vm.loaded("base")
        vm.saveDocument("singbox/dns", "{}", "base", onComplete = { oldCallbacks++ })
        withTimeout(5_000) { entered.await() }
        val latest = CompletableDeferred<SingBoxDocumentSaveResult>()
        vm.saveDocument("singbox/dns", "{\"new\":true}", "save-1", onComplete = { latest.complete(it) })
        release.complete(Unit)
        assertTrue(withTimeout(5_000) { latest.await() }.success)
        finishOperations()
        assertEquals(2, writes.get())
        assertEquals(1, maxActiveWrites.get())
        assertEquals(0, oldCallbacks)
        assertEquals("save-2", vm.state.value.activeDocumentRevision)
        assertEquals("{\"new\":true}", vm.state.value.activeDocumentContent)
    }
}
