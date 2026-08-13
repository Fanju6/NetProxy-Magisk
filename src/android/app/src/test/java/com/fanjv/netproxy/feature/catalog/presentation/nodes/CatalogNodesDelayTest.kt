package com.fanjv.netproxy.feature.catalog.presentation.nodes

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class CatalogNodesDelayTest {
    @Test
    fun `auto delay uses fastest successful node`() {
        val measured = mapOf(
            "group/slow" to "188",
            "group/fast" to "72"
        )

        assertEquals(
            "72",
            groupAutoDelay(measured, listOf("group/slow", "group/timeout", "group/fast"))
        )
    }

    @Test
    fun `auto delay is absent when every node timed out`() {
        assertNull(groupAutoDelay(emptyMap(), listOf("group/one", "group/two")))
    }
}
