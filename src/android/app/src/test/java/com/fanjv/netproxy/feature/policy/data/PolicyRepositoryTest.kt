package com.fanjv.netproxy.feature.policy.data

import com.fanjv.netproxy.core.api.ModuleApiClient
import com.fanjv.netproxy.core.api.TestApiTransport
import com.fanjv.netproxy.core.api.apiSuccess
import com.fanjv.netproxy.feature.policy.model.PolicyModel
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class PolicyRepositoryTest {
    @Test fun createsPackageOnlyPolicyFollowingProxyWithoutLocalNodes() = runBlocking {
        val repository = PolicyRepository(ModuleApiClient(TestApiTransport { request ->
            assertEquals("POST", request.method)
            assertEquals("/policies", request.path)
            assertEquals("hills", request.text("name"))
            assertEquals("proxy", request.text("mode"))
            assertFalse(request.body.containsKey("group_id"))
            assertEquals(listOf("com.mountains.hills"), request.body.getValue("package_names").jsonArray.map { it.jsonPrimitive.content })
            assertTrue(request.body.getValue("domain_suffixes").jsonArray.isEmpty())
            assertTrue(request.body.getValue("ports").jsonArray.isEmpty())
            apiSuccess("""{"policy":{"id":"custom-hills","name":"hills","mode":"proxy","package_names":["com.mountains.hills"]},"persisted":true,"runtime_synced":false}""")
        }))

        val saved = repository.create("hills", listOf("com.mountains.hills"), emptyList(), emptyList())
        assertEquals("custom-hills", saved.id)
        assertEquals("proxy", saved.mode)
        assertEquals(listOf("com.mountains.hills"), saved.packageNames)
        assertTrue(saved.persisted)
        assertFalse(saved.runtimeSynced)
    }

    @Test fun editingDefinitionReturnsSavedPolicyAndKeepsTarget() = runBlocking {
        val repository = PolicyRepository(ModuleApiClient(TestApiTransport { request ->
            assertEquals("PUT", request.method)
            assertEquals("/policies/custom-hills", request.path)
            assertEquals("subscription-id", request.text("group_id"))
            assertEquals("auto", request.text("mode"))
            assertEquals("jp", request.text("region"))
            apiSuccess("""{"policy":{"id":"custom-hills","name":"hills edited","mode":"auto","group_id":"subscription-id","region":"jp"},"persisted":true,"runtime_synced":true}""")
        }))
        val saved = repository.updateDefinition(
            PolicyModel("custom-hills", "hills", groupId = "subscription-id", mode = "auto", region = "jp"),
            "hills edited", listOf("com.mountains.hills"), emptyList(), emptyList(),
        )
        assertEquals("hills edited", saved.name)
        assertEquals("jp", saved.region)
        assertTrue(saved.runtimeSynced)
    }

    @Test fun listDecodesPolicyGroupsAndSaveUsesStableContract() = runBlocking {
        val calls = mutableListOf<String>()
        val client = ModuleApiClient(TestApiTransport { request ->
            calls += "${request.method} ${request.path}"
            when (request.method to request.path) {
                "GET" to "/policies" -> apiSuccess("""
                    {"schema":1,"policies":[{"id":"ai","name":"AI","group_id":"default","mode":"auto","groups":[{"group":{"id":"default","name":"Local","type":"local"},"nodes":[{"tag":"Tokyo"}]}]}]}
                """.trimIndent())
                "PUT" to "/policies/ai" -> {
                    assertEquals("default", request.text("group_id"))
                    assertEquals("Tokyo", request.text("node_tag"))
                    assertEquals("manual", request.text("mode"))
                    apiSuccess("""{"policy":{"id":"ai"},"persisted":true,"runtime_synced":false}""")
                }
                else -> error("Unexpected policy request: $request")
            }
        })
        val repository = PolicyRepository(client)
        val policy = repository.list().single()
        assertEquals("ai", policy.id)
        assertEquals("Tokyo", policy.groups.single().nodes.single().tag)
        repository.save(policy.copy(groupId = "default", nodeTag = "Tokyo", mode = "manual"))
        assertEquals(listOf("GET /policies", "PUT /policies/ai"), calls)
    }

    @Test fun urlTestDecodesDelayGroups() = runBlocking {
        val client = ModuleApiClient(TestApiTransport { request ->
            assertEquals("POST", request.method)
            assertEquals("/policies/ai/url-test", request.path)
            apiSuccess("""{"target":"auto","groups":[{"tag":"default","items":[{"tag":"Tokyo","url_test_delay":70}]}]}""")
        })
        val result = PolicyRepository(client).test(PolicyModel("ai", "AI"))
        assertEquals("auto", result.target)
        assertEquals(70, result.groups.single().items.single().urlTestDelay)
        assertTrue(result.groups.single().items.single().urlTestTime == null)
    }

    @Test fun regionAutoSavesAnEmptyNodeTagAndStringRegion() = runBlocking {
        val client = ModuleApiClient(TestApiTransport { request ->
            assertEquals("PUT", request.method)
            assertEquals("/policies/ai", request.path)
            assertEquals("default", request.text("group_id"))
            assertEquals("", request.text("node_tag"))
            assertEquals("auto", request.text("mode"))
            assertEquals("us", request.text("region"))
            apiSuccess("""{"policy":{"id":"ai"},"persisted":true,"runtime_synced":false}""")
        })

        PolicyRepository(client).save(PolicyModel("ai", "AI", groupId = "default", mode = "auto", region = "us"))
    }
}
