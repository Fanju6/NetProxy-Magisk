package com.fanjv.netproxy.feature.catalog.presentation.subscriptions

import com.fanjv.netproxy.core.ui.UiText
import com.fanjv.netproxy.feature.catalog.data.subscriptionEditArguments
import com.fanjv.netproxy.feature.catalog.model.SubscriptionDraft
import com.fanjv.netproxy.feature.catalog.model.SubscriptionEditorState
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import org.junit.Assert.*
import org.junit.Test

class SubscriptionEditorModelTest {
    private val original = SubscriptionEditorState("sample", "Sample", "https://example.invalid/a")
    private val failure = UiText.Plain("persisted failure")

    @Test fun committedFailureAdvancesBaselineSoRevertingUrlIsNotANoop() {
        val submitted = SubscriptionEditorUiState(
            id = original.id, original = original, draft = original.toDraft().copy(url = "https://example.invalid/b")
        )
        val disk = original.copy(url = submitted.draft.url)
        val state = submitted.copy(error = failure, persisted = true, baselineReadRequired = true).rebase(submitted, disk)
        assertEquals(disk, state.original)
        assertEquals(failure, state.error)
        assertFalse(state.saved)
        assertFalse(state.baselineReadRequired)
        assertEquals(
            listOf("sub", "edit", "--url", original.url, original.id),
            subscriptionEditArguments(state.id, state.original!!, state.draft.copy(url = original.url), null)
        )
    }

    @Test fun persistedAddBecomesEditAndKeepsLaterDraftAndError() {
        val submitted = SubscriptionEditorUiState(draft = SubscriptionDraft("", "https://example.invalid/a"))
        val disk = original.copy(name = "Assigned name", customHeaders = JsonObject(mapOf("X-Sample" to JsonPrimitive("disk"))))
        val current = submitted.copy(
            draft = submitted.draft.copy(url = "https://example.invalid/later", timeoutSeconds = 90),
            headersText = "X-Sample: later", error = failure, persisted = true
        )
        val state = current.rebase(submitted, disk)
        assertEquals(original.id, state.id)
        assertEquals(disk, state.original)
        assertEquals("Assigned name", state.draft.name)
        assertEquals(current.draft.url, state.draft.url)
        assertEquals(90, state.draft.timeoutSeconds)
        assertEquals(current.headersText, state.headersText)
        assertEquals(failure, state.error)
        assertFalse(state.saved)
        assertFalse(state.sameInput(submitted))
    }

    @Test fun editorArgumentsOnlyContainActuallyChangedFields() {
        assertEquals(listOf("sub", "edit", "sample"), subscriptionEditArguments(original.id, original, original.toDraft(), null))
        assertEquals(
            listOf("sub", "edit", "--name", "New name", "sample"),
            subscriptionEditArguments(original.id, original, original.toDraft().copy(name = "New name"), null)
        )
        val disk = original.copy(customHeaders = JsonObject(mapOf("X-Sample" to JsonPrimitive("fixture"))))
        assertEquals(
            listOf("sub", "edit", "--headers-file", "/fixture/headers.json", "sample"),
            subscriptionEditArguments(disk.id, disk, disk.toDraft().copy(customHeaders = emptyMap()), "/fixture/headers.json")
        )
    }

    @Test fun recoveringFailedReadKeepsAllUnsubmittedInputs() {
        val state = SubscriptionEditorUiState(
            id = "sample", persisted = true, baselineReadRequired = true,
            draft = SubscriptionDraft("", "https://example.invalid/new", timeoutSeconds = 90),
            headersText = "X-Sample: unsaved", error = failure
        )
        val result = state.recoverBaseline(original)
        assertEquals(original, result.original)
        assertEquals(state.draft.copy(name = original.name), result.draft)
        assertEquals(state.headersText, result.headersText)
        assertEquals(failure, result.error)
        assertFalse(result.baselineReadRequired)
    }

    @Test fun runtimeOutcomeUsesOnlyCurrentContractAndAcceptsNonObjectErrorData() {
        val outcome = Json.parseToJsonElement("""{"group_id":"sample","persisted":true,"runtime_sync_state":"pending","runtime_sync_pending":true}""").subscriptionOutcome()
        assertEquals(SubscriptionRuntimeOutcome("sample", true, "pending", true), outcome)
        assertEquals(SubscriptionRuntimeOutcome(), JsonPrimitive("failure").subscriptionOutcome())
    }

    @Test fun customIntervalsKeepExactSecondsAndEveryPresetKeepsItsIndex() {
        val presets = listOf(900L, 3600L, 21600L, 43200L, 86400L, 259200L, 604800L)
        for (value in presets) assertEquals(presets, subscriptionIntervalOptions(value))
        for (value in listOf(901L, 5400L, 86501L)) {
            val options = subscriptionIntervalOptions(value)
            assertEquals(presets + value, options)
            assertEquals(value, options[options.indexOf(value)])
        }
    }
}
