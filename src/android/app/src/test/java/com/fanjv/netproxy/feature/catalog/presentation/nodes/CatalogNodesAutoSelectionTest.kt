package com.fanjv.netproxy.feature.catalog.presentation.nodes

import com.fanjv.netproxy.feature.catalog.model.CatalogGroupSummary
import com.fanjv.netproxy.feature.catalog.model.CatalogNode
import com.fanjv.netproxy.feature.catalog.model.CatalogNodeGroup
import com.fanjv.netproxy.feature.catalog.model.CurrentNodeSelection
import org.junit.Assert.assertEquals
import org.junit.Test

class CatalogNodesAutoSelectionTest {
    @Test
    fun `resolves selected outbound to a node in the active group`() {
        val group = group("active", "node/with-slash")
        val selection = CurrentNodeSelection(
            activeGroupId = "active",
            selectorMode = "urltest",
            activeGroupRuntimeTag = "Runtime [active]",
            runtimeSelected = "Runtime [active]/node/with-slash"
        )

        assertEquals("node/with-slash", selectedAutoNodeTag(group, selection))
    }

    @Test
    fun `does not show selection for another group or manual mode`() {
        val group = group("active", "node")
        val selection = CurrentNodeSelection(
            activeGroupId = "other",
            selectorMode = "urltest",
            activeGroupRuntimeTag = "Runtime",
            runtimeSelected = "Runtime/node"
        )

        assertEquals("", selectedAutoNodeTag(group, selection))
        assertEquals(
            "",
            selectedAutoNodeTag(group, selection.copy(activeGroupId = "active", selectorMode = "manual"))
        )
    }

    @Test
    fun `ignores runtime selections that do not map to a catalog node`() {
        val group = group("active", "node")
        val selection = CurrentNodeSelection(
            activeGroupId = "active",
            selectorMode = "urltest",
            activeGroupRuntimeTag = "Runtime",
            runtimeSelected = "Runtime/removed-node"
        )

        assertEquals("", selectedAutoNodeTag(group, selection))
    }

    private fun group(id: String, vararg nodeTags: String) = CatalogNodeGroup(
        group = CatalogGroupSummary(id = id, name = id, type = "local"),
        nodes = nodeTags.map { CatalogNode(tag = it) }
    )
}
