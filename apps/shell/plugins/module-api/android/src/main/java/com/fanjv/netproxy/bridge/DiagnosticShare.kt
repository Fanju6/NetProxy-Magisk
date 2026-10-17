package com.fanjv.netproxy.bridge

import android.app.Activity
import android.content.ClipData
import android.content.Intent
import androidx.core.content.FileProvider
import com.fanjv.netproxy.core.api.ModuleApiTransport
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.io.File
import java.util.concurrent.atomic.AtomicBoolean

class DiagnosticFileProvider : FileProvider()

/** Only a fixed server export into a narrowly exposed private cache directory. */
internal object DiagnosticShare {
    internal class Prepared(private val open: () -> Unit, private val cleanup: () -> Unit) {
        private val settled = AtomicBoolean(false)

        fun deliver() {
            if (!settled.compareAndSet(false, true)) return
            try { open() } catch (error: Exception) { cleanup(); throw error }
        }

        fun discard() {
            if (settled.compareAndSet(false, true)) cleanup()
        }
    }

    suspend fun prepare(owner: Activity, transport: ModuleApiTransport): Prepared {
        var pending: File? = null
        try {
            return withContext(Dispatchers.IO) {
                val directory = File(owner.cacheDir, "netproxy-diagnostics")
                if (!directory.isDirectory && !directory.mkdirs()) throw BridgeFailure("api.native_failed")
                val cutoff = System.currentTimeMillis() - 24 * 60 * 60 * 1000
                directory.listFiles()?.filter { it.isFile && it.name.startsWith("NetProxy-") && it.lastModified() < cutoff }?.forEach { it.delete() }
                if ((directory.listFiles()?.size ?: 0) >= 4) throw BridgeFailure("api.busy")
                val file = File.createTempFile("NetProxy-", ".tar.gz", directory)
                pending = file
                transport.download("/diagnostics/export", emptyMap(), file)
                if (file.length() <= 0 || file.length() > 64L * 1024 * 1024) throw BridgeFailure("api.body_too_large")
                file.inputStream().use { if (it.read() != 0x1f || it.read() != 0x8b) throw BridgeFailure("api.invalid_response") }
                Prepared(open = {
                    if (owner.isDestroyed || owner.isFinishing) throw BridgeFailure("api.destroyed")
                    val uri = FileProvider.getUriForFile(owner, owner.packageName + ".netproxy.diagnostics", file)
                    val send = Intent(Intent.ACTION_SEND).apply {
                        type = "application/gzip"
                        putExtra(Intent.EXTRA_STREAM, uri)
                        clipData = ClipData.newRawUri("NetProxy diagnostics", uri)
                        addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
                    }
                    owner.startActivity(Intent.createChooser(send, null))
                }, cleanup = { file.delete() })
            }
        } catch (error: Exception) {
            pending?.delete()
            throw error
        }
    }
}
