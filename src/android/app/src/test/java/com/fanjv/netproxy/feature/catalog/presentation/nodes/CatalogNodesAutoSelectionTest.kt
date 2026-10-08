package com.fanjv.netproxy.feature.catalog.presentation.nodes

import com.fanjv.netproxy.core.ui.UiText
import com.fanjv.netproxy.feature.catalog.model.CatalogGroupSummary
import com.fanjv.netproxy.feature.catalog.model.CatalogNode
import com.fanjv.netproxy.feature.catalog.model.CatalogNodeGroup
import com.fanjv.netproxy.feature.catalog.model.CatalogNodesSnapshot
import com.fanjv.netproxy.feature.catalog.model.CurrentNodeSelection
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Test

class CatalogNodesAutoSelectionTest {
    @Test
    fun `refreshes committed selection without clearing an unconsumed failure`() {
        val message = UiText.Plain("fixture runtime failure")
        val previous = CatalogNodesUiState(
            selectedGroupId = "removed",
            error = message,
            noticeId = 7,
            loading = true
        )
        val snapshot = CatalogNodesSnapshot(
            groups = listOf(group("default", "NODE")),
            selection = CurrentNodeSelection(activeGroupId = "default", selectorMode = "urltest")
        )
        val refreshed = previous.withSnapshot(snapshot)

        assertEquals(snapshot.groups, refreshed.groups)
        assertEquals(snapshot.selection, refreshed.selection)
        assertEquals("default", refreshed.selectedGroupId)
        assertEquals(message, refreshed.error)
        assertEquals(7L, refreshed.noticeId)
        assertFalse(refreshed.loading)
        assertEquals("", refreshed.withSnapshot(CatalogNodesSnapshot()).selectedGroupId)
        assertEquals("default", refreshed.withSnapshot(snapshot.copy(selection = CurrentNodeSelection(activeGroupId = "other"))).selectedGroupId)
    }

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
