package com.fanjv.netproxy.feature.traffic.data

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.Json
import java.io.File

internal class CapturePageCache(
    private val dir: File,
    private val json: Json,
    private val budget: Long = 8L shl 20,
) {
    private val lock = Any()
    private val batchPattern = Regex("[A-Z2-7]{26}")
    private val cursorPattern = Regex("[0-9]{1,20}")

    suspend fun read(batch: String, oldest: String, before: String): CapturePage? = onDisk {
        val file = pageFile(batch, oldest, before) ?: return@onDisk null
        if (!file.isFile || file.length() !in 1..budget) return@onDisk null
        runCatching { json.decodeFromString<CapturePage>(file.readText()) }.getOrNull()
            ?.takeIf { it.batch == batch && it.oldest == oldest }?.also {
                file.setLastModified(System.currentTimeMillis())
            }
    }

    suspend fun write(batch: String, oldest: String, before: String, page: CapturePage) = onDisk {
        val file = pageFile(batch, oldest, before) ?: return@onDisk
        runCatching {
            val data = json.encodeToString(CapturePage.serializer(), page).encodeToByteArray()
            if (data.size > budget) return@runCatching
            if (!dir.isDirectory && !dir.mkdirs()) return@runCatching
            val prefix = "${batch}_${oldest}_"
            dir.listFiles().orEmpty().filter {
                it.name.startsWith("${batch}_") && !it.name.startsWith(prefix)
            }.forEach { it.delete() }
            val temp = File.createTempFile("page-", ".tmp", dir)
            try {
                temp.writeBytes(data)
                if (temp.renameTo(file)) prune()
            } finally {
                temp.delete()
            }
        }
        Unit
    }

    suspend fun retain(batches: Set<String>) = onDisk {
        dir.listFiles().orEmpty().filter { it.name.substringBefore('_') !in batches }.forEach { it.delete() }
    }

    suspend fun invalidate(batch: String? = null) = onDisk {
        dir.listFiles().orEmpty().filter { batch == null || it.name.substringBefore('_') == batch }.forEach { it.delete() }
    }

    private fun pageFile(batch: String, oldest: String, before: String): File? =
        if (batchPattern.matches(batch) && cursorPattern.matches(oldest) && cursorPattern.matches(before)) {
            File(dir, "${batch}_${oldest}_$before.json")
        } else null

    private fun prune() {
        var bytes = 0L
        dir.listFiles().orEmpty().sortedByDescending { it.lastModified() }.forEachIndexed { index, file ->
            bytes += file.length()
            if (index >= 128 || bytes > budget) file.delete()
        }
    }

    private suspend fun <T> onDisk(action: () -> T): T = withContext(Dispatchers.IO) { synchronized(lock) { action() } }
}
