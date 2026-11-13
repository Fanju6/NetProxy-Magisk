package com.fanjv.netproxy.feature.proxy

import com.fanjv.netproxy.core.api.ModuleApiClient
import com.fanjv.netproxy.core.api.ModuleHttpResponse
import com.fanjv.netproxy.core.api.TestApiTransport
import com.fanjv.netproxy.core.api.apiSuccess
import com.fanjv.netproxy.feature.proxy.data.PolicyGroupDocument
import com.fanjv.netproxy.feature.proxy.data.PolicyRepository
import com.fanjv.netproxy.feature.proxy.data.parsePolicySection
import com.fanjv.netproxy.feature.proxy.presentation.PolicyGroupEditorViewModel
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.job
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class PolicyGroupEditorTest {
    @Test fun deletesOnlyTheConfirmedGroupWithTheCurrentRevision() = runBlocking {
        var writes = 0
        val original = parsePolicySection(source, "outbounds")
        val client = ModuleApiClient(TestApiTransport { request ->
            if (request.mutation) {
                writes++
                request.assertConfig("PUT", "singbox/outbounds", "r1")
                assertEquals(listOf(original[0], original[2]), parsePolicySection(request.text("content"), "outbounds"))
                apiSuccess("""{"revision":"r2"}""")
            } else {
                request.assertConfig("GET", "singbox/outbounds")
                configResponse(source)
            }
        })
        PolicyRepository(client).deleteGroup("Proxy", "selector")
        assertEquals(1, writes)
    }

    @Test fun refusesToDeleteAnOutboundThatNoLongerMatchesTheConfirmedType() = runBlocking {
        val client = ModuleApiClient(TestApiTransport { request ->
            check(!request.mutation)
            configResponse(source)
        })
        val repository = PolicyRepository(client)
        assertTrue(runCatching { repository.deleteGroup("Proxy", "urltest") }.exceptionOrNull() is IllegalArgumentException)
        repository.deleteGroup("already removed", "selector")
    }

    @Test fun addingAutoPreservesExistingDefaultsAndOtherOutbounds() {
        val original = parsePolicySection(source, "outbounds")
        val tags = listOf("direct", "Proxy", "block")
        val document = PolicyGroupDocument(original, "r1", "Proxy", tags, emptyList())
        assertTrue(document.canAddAuto)

        val updated = parsePolicySection(document.updated(document.initial.copy(addAuto = true)), "outbounds")
        assertEquals(original[0], updated[0])
        assertEquals(original[2], updated[2])
        assertEquals(original[1]["default"], updated[1]["default"])
        assertEquals(original[1]["interrupt_exist_connections"], updated[1]["interrupt_exist_connections"])
        assertEquals(JsonArray(listOf(JsonPrimitive("direct"), JsonPrimitive("Auto"))), updated[1]["outbounds"])
        assertEquals(parsePolicySection(
            """{"outbounds":[{"type":"urltest","tag":"Auto","outbounds":["block"],"use_all_providers":true}]}""",
            "outbounds",
        ).single(), updated.last())
        assertFalse(PolicyGroupDocument(updated, "r2", "Proxy", tags + "Auto", emptyList()).canAddAuto)
        assertFalse(PolicyGroupDocument(original, "r1", "Proxy", tags + "Auto", emptyList()).canAddAuto)
        assertFalse(PolicyGroupDocument(original.take(2), "r1", "Proxy", tags.take(2), emptyList()).canAddAuto)
    }

    @Test fun updatesOnlyTheTargetGroupAndPreservesNativeFields() {
        val original = parsePolicySection(source, "outbounds")
        val document = PolicyGroupDocument(original, "r1", "Proxy", listOf("Proxy", "direct"), emptyList())
        val result = parsePolicySection(
            document.updated(document.initial.copy(providers = listOf("Local"), include = "JP")),
            "outbounds",
        )

        assertEquals(original[0], result[0])
        assertEquals(original[1]["interrupt_exist_connections"], result[1]["interrupt_exist_connections"])
        assertEquals(original[1]["outbounds"], result[1]["outbounds"])
        assertEquals(JsonPrimitive("Proxy"), result[1]["tag"])
        assertEquals(JsonPrimitive("JP"), result[1]["include"])
    }

    @Test fun createsAGroupThroughTheRevisionCheckedOutboundSection() = runBlocking {
        var writes = 0
        val client = ModuleApiClient(TestApiTransport { request ->
            if (request.mutation) {
                writes++
                request.assertConfig("PUT", "singbox/outbounds", "r1")
                val groups = parsePolicySection(request.text("content"), "outbounds")
                assertEquals(JsonPrimitive("Auto"), groups.last()["tag"])
                assertEquals(JsonPrimitive("urltest"), groups.last()["type"])
                assertEquals(JsonPrimitive("3m"), groups.last()["interval"])
                apiSuccess("""{"revision":"r2"}""")
            } else when (request.path) {
                "/config" -> when (request.target) {
                    "singbox/outbounds" -> configResponse(source)
                    "singbox/endpoints" -> configResponse("""{"endpoints":[{"tag":"WireGuard"}]}""")
                    "singbox/providers" -> configResponse("""{"providers":[{"tag":"native"}]}""")
                    else -> error("Unexpected target")
                }
                "/subscriptions" -> apiSuccess(
                    """[{"id":"default","name":"Local","type":"local","node_count":2,"runtime_tag":"Local"}]"""
                )
                else -> error("Unexpected request")
            }
        })
        val repository = PolicyRepository(client)
        val document = repository.openEditor(null)
        assertEquals(listOf("direct", "Proxy", "block", "WireGuard"), document.memberTags)
        assertEquals(listOf("native", "Local"), document.providerChoices)
        repository.saveGroup(document, document.initial.copy(tag = "Auto", type = "urltest",
            outbounds = listOf("direct"), testInterval = "3m"))
        assertEquals(1, writes)
    }

    @Test fun aRevisionConflictKeepsTheDraftAndDoesNotReplayTheWrite() = runBlocking {
        val scope = CoroutineScope(coroutineContext + SupervisorJob(coroutineContext.job))
        var writes = 0
        val client = ModuleApiClient(TestApiTransport { request ->
            when {
                request.mutation -> {
                    writes++
                    request.assertConfig("PUT", "singbox/outbounds", "r1")
                    val groups = parsePolicySection(request.text("content"), "outbounds")
                    assertEquals(JsonArray(listOf(JsonPrimitive("direct"), JsonPrimitive("Auto"))), groups[1]["outbounds"])
                    assertEquals(JsonPrimitive("Auto"), groups.last()["tag"])
                    ModuleHttpResponse(409, """{"schema":1,"ok":false,"code":"config.conflict","message":"Reload","data":null}""")
                }
                request.path == "/subscriptions" -> apiSuccess("[]")
                request.target == "singbox/outbounds" -> configResponse(source)
                else -> configResponse("{}")
            }
        }, scope)
        val vm = PolicyGroupEditorViewModel(PolicyRepository(client), scope)
        try {
            vm.openEditor("Proxy")
            withTimeout(5_000) { vm.editor.first { it?.loading == false } }
            vm.changeGroupDraft { it.copy(addAuto = true) }
            vm.saveEditor()
            val state = withTimeout(5_000) { vm.editor.first { it?.conflict == true } }!!
            assertTrue(state.draft?.addAuto == true)
            assertFalse(state.canSave)
            vm.saveEditor()
            assertTrue(writes == 1)
        } finally {
            scope.cancel()
        }
    }

    private fun configResponse(content: String) =
        apiSuccess("""{"revision":"r1","content":${JsonPrimitive(content)}}""")

    private val source = """
        {"outbounds":[{"type":"direct","tag":"direct"},
          {"type":"selector","tag":"Proxy","outbounds":["direct"],"default":"direct","interrupt_exist_connections":true},
          {"type":"block","tag":"block"}]}
    """.trimIndent()
}
