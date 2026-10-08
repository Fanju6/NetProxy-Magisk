package com.fanjv.netproxy.feature.kernel.presentation

import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonObject

internal suspend fun parseEditorJson(rawJson: String): JsonObject = withContext(Dispatchers.Default) {
    ensureActive()
    singBoxSchemaJson.parseToJsonElement(rawJson).jsonObject.also { ensureActive() }
}

internal suspend fun formatEditorJson(document: JsonObject): String = withContext(Dispatchers.Default) {
    ensureActive()
    editorPrettyJson.encodeToString(document).also { ensureActive() }
}

internal suspend fun validateEditorJson(
    rawJson: String,
    validator: SingBoxSchemaValidator?,
    text: SchemaText,
): SingBoxSchemaValidationResult = withContext(Dispatchers.Default) {
    ensureActive()
    val document = try {
        parseEditorJson(rawJson)
    } catch (error: CancellationException) {
        throw error
    } catch (error: IllegalArgumentException) {
        return@withContext SingBoxSchemaValidationResult.Invalid(listOf(jsonSyntaxIssue(rawJson, error, text)))
    }
    validator?.validate(rawJson, document) ?: SingBoxSchemaValidationResult.Valid
}

private val editorPrettyJson = Json(singBoxSchemaJson) { prettyPrint = true }
