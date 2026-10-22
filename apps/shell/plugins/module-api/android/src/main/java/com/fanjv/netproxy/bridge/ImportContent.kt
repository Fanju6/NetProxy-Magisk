package com.fanjv.netproxy.bridge

import java.io.ByteArrayOutputStream
import java.io.InputStream
import java.nio.ByteBuffer
import java.nio.charset.CodingErrorAction
import java.util.UUID

internal const val IMPORT_LIMIT = 1024 * 1024

internal data class ImportContent(val id: String, val name: String, val content: String)

internal fun importContent(name: String, content: String): ImportContent {
    if (content.toByteArray(Charsets.UTF_8).size > IMPORT_LIMIT) throw BridgeFailure("api.body_too_large")
    if (content.isBlank() || content.any { it.isISOControl() && it !in "\n\r\t" })
        throw BridgeFailure("api.import_invalid")
    return ImportContent(UUID.randomUUID().toString(), name.filterNot { it.isISOControl() || it in "/\\:" }.take(160), content)
}

internal fun readImportContent(name: String, stream: InputStream): ImportContent {
    val output = ByteArrayOutputStream()
    val buffer = ByteArray(8192)
    while (true) {
        val count = stream.read(buffer)
        if (count < 0) break
        if (output.size() + count > IMPORT_LIMIT) throw BridgeFailure("api.body_too_large")
        output.write(buffer, 0, count)
    }
    val content = try {
        Charsets.UTF_8.newDecoder().onMalformedInput(CodingErrorAction.REPORT)
            .onUnmappableCharacter(CodingErrorAction.REPORT).decode(ByteBuffer.wrap(output.toByteArray())).toString()
    } catch (_: Exception) { throw BridgeFailure("api.import_invalid") }
    return importContent(name, content.removePrefix("\uFEFF"))
}

/** Memory only: consuming a preview never executes an import or survives process death. */
internal class ImportInbox {
    private val pending = ArrayDeque<Result<ImportContent>>()
    private var overflow = false
    private var active: ImportContent? = null
    private var presentedTo: Long? = null

    @Synchronized fun clear() { pending.clear(); overflow = false; active = null; presentedTo = null }

    @Synchronized fun hasActive(): Boolean = active != null

    @Synchronized fun claim(value: ImportContent, document: Long): ImportContent {
        if (active != null) throw BridgeFailure("api.busy")
        active = value
        presentedTo = document
        return value
    }

    @Synchronized fun complete(id: String) {
        if (active?.id == id) { active = null; presentedTo = null }
    }

    @Synchronized fun offer(value: Result<ImportContent>) {
        if (pending.size < 4) pending.addLast(value) else overflow = true
    }

    @Synchronized fun take(document: Long = 0): ImportContent? {
        active?.let {
            if (presentedTo == document) return null
            presentedTo = document
            return it
        }
        if (pending.isNotEmpty()) return claim(pending.removeFirst().getOrThrow(), document)
        if (overflow) {
            overflow = false
            throw BridgeFailure("api.busy")
        }
        return null
    }
}
