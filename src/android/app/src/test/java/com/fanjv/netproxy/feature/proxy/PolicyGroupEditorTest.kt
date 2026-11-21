package com.fanjv.netproxy.feature.proxy

import com.fanjv.netproxy.core.api.ModuleApiClient
import com.fanjv.netproxy.core.api.ModuleApiException
import com.fanjv.netproxy.core.api.ModuleHttpResponse
import com.fanjv.netproxy.core.api.TestApiTransport
import com.fanjv.netproxy.core.api.apiSuccess
import com.fanjv.netproxy.feature.proxy.data.PolicyGroupDocument
import com.fanjv.netproxy.feature.proxy.data.PolicyRepository
import com.fanjv.netproxy.feature.proxy.data.parsePolicySection
import com.fanjv.netproxy.feature.proxy.data.text
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

    @Test fun createsOneSelectorPerEligibleSubscriptionInASingleWrite() = runBlocking {
        val original = parsePolicySection(source, "outbounds")
        val existing = original + parsePolicySection("""
            {"outbounds":[{"tag":"Existing","type":"selector","providers":"Work"},
              {"tag":"Office (Subscription) (2)","type":"direct"}]}
        """.trimIndent(), "outbounds")
        var content = """{"outbounds":${JsonArray(existing)}}"""
        var writes = 0
        val client = ModuleApiClient(TestApiTransport { request ->
            if (request.mutation) {
                writes++
                request.assertConfig("PUT", "singbox/outbounds", "r1")
                content = request.text("content")
                val groups = parsePolicySection(content, "outbounds")
                assertEquals(existing, groups.take(existing.size))
                val added = groups.drop(existing.size)
                assertEquals(listOf("Office (Subscription) (3)", "Office (2) (Subscription) (2)"), added.map { it.text("tag") })
                assertEquals(listOf("Office", "Office (2)"), added.map {
                    assertEquals(setOf("tag", "type", "providers"), it.keys)
                    assertEquals("selector", it.text("type"))
                    ((it.getValue("providers") as JsonArray).single() as JsonPrimitive).content
                })
                apiSuccess("""{"revision":"r2"}""")
            } else when (request.path) {
                "/config" -> when (request.target) {
                    "singbox/outbounds" -> configResponse(content)
                    "singbox/endpoints" -> configResponse("""{"endpoints":[{"tag":"Office (Subscription)"}]}""")
                    "singbox/providers" -> configResponse("{}")
                    else -> error("Unexpected target")
                }
                "/subscriptions" -> apiSuccess("""
                    [
                      {"id":"s1","name":"Office","type":"subscription","node_count":2,"runtime_tag":"Office"},
                      {"id":"s2","name":"Office (2)","type":"subscription","node_count":1,"runtime_tag":"Office (2)"},
                      {"id":"s3","name":"Work","type":"subscription","node_count":1,"runtime_tag":"Work"},
                      {"id":"s4","name":"Empty","type":"subscription","node_count":0,"runtime_tag":"Empty"},
                      {"id":"local","name":"Office (2) (Subscription)","type":"local","node_count":0,"runtime_tag":"Office (2) (Subscription)"},
                      {"id":"local2","name":"Local","type":"local","node_count":3,"runtime_tag":"Local"}
                    ]
                """.trimIndent())
                else -> error("Unexpected request")
            }
        })
        val repository = PolicyRepository(client)
        assertEquals(2, repository.createSubscriptionGroups("Subscription"))
        assertEquals(0, repository.createSubscriptionGroups("Another locale"))
        assertEquals(1, writes)
    }

    @Test fun doesNotReplaySubscriptionGroupCreationOnRevisionConflict() = runBlocking {
        var writes = 0
        val client = ModuleApiClient(TestApiTransport { request ->
            when {
                request.mutation -> {
                    writes++
                    request.assertConfig("PUT", "singbox/outbounds", "r1")
                    ModuleHttpResponse(409, """{"schema":1,"ok":false,"code":"config.conflict","message":"Reload","data":null}""")
                }
                request.path == "/subscriptions" -> apiSuccess(
                    """[{"id":"s1","name":"Work","type":"subscription","node_count":1,"runtime_tag":"Work"}]"""
                )
                request.target == "singbox/outbounds" -> configResponse(source)
                else -> configResponse("{}")
            }
        })
        val error = runCatching { PolicyRepository(client).createSubscriptionGroups("Subscription") }.exceptionOrNull()
        assertEquals("config.conflict", (error as? ModuleApiException)?.resultCode)
        assertEquals(1, writes)
    }

    @Test fun convertsAnExistingSelectorWithTheSameTagAndRevision() = runBlocking {
        val scope = CoroutineScope(coroutineContext + SupervisorJob(coroutineContext.job))
        val original = parsePolicySection(source, "outbounds")
        var writes = 0
        val client = ModuleApiClient(TestApiTransport { request ->
            when {
                request.mutation -> {
                    writes++
                    request.assertConfig("PUT", "singbox/outbounds", "r1")
                    val groups = parsePolicySection(request.text("content"), "outbounds")
                    assertEquals(original.first(), groups.first())
                    assertEquals(original.last(), groups.last())
                    assertEquals(original[1]["tag"], groups[1]["tag"])
                    assertEquals(original[1]["outbounds"], groups[1]["outbounds"])
                    assertEquals(original[1]["interrupt_exist_connections"], groups[1]["interrupt_exist_connections"])
                    assertEquals(JsonPrimitive("urltest"), groups[1]["type"])
                    assertEquals(JsonPrimitive("3m"), groups[1]["interval"])
                    assertFalse(groups[1].containsKey("default"))
                    apiSuccess("""{"revision":"r2"}""")
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
            vm.changeGroupDraft { it.copy(type = "urltest", testInterval = "3m") }
            assertTrue(vm.editor.value?.canSave == true)
            vm.saveEditor()
            withTimeout(5_000) { vm.editor.first { it?.saved == true } }
            assertEquals(1, writes)
        } finally {
            scope.cancel()
        }
    }

    @Test fun dropsOnlyIncompatibleOptionsWhenSwitchingType() {
        val original = parsePolicySection("""
            {"outbounds":[{"type":"urltest","tag":"Work","outbounds":["direct"],"providers":["Local"],
                "include":"JP","url":"https://example.invalid","interval":"3m","tolerance":50,
                "idle_timeout":"10m","fallback":{"enabled":true},"interrupt_exist_connections":true}]}
        """.trimIndent(), "outbounds")
        val doc = PolicyGroupDocument(original, "r1", "Work", listOf("direct", "Work"), listOf("Local"))
        assertEquals(original, parsePolicySection(doc.updated(doc.initial), "outbounds"))
        val next = parsePolicySection(
            doc.updated(doc.initial.copy(type = "selector", defaultMember = "direct")), "outbounds",
        ).single()
        assertEquals(JsonPrimitive("selector"), next["type"])
        assertEquals(JsonPrimitive("direct"), next["default"])
        for (field in listOf("tag", "outbounds", "providers", "include", "interrupt_exist_connections")) {
            assertEquals(original.single()[field], next[field])
        }
        assertTrue(listOf("url", "interval", "tolerance", "idle_timeout", "fallback").none(next::containsKey))
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
