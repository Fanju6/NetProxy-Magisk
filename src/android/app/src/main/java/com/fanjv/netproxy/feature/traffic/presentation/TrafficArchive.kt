package com.fanjv.netproxy.feature.traffic.presentation

import android.content.ClipData
import android.content.Intent
import android.provider.OpenableColumns
import android.widget.Toast
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.core.content.FileProvider
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.api.ModuleApiException
import com.fanjv.netproxy.core.ui.component.AppDialog
import com.fanjv.netproxy.core.ui.component.AppDialogButton
import com.fanjv.netproxy.core.ui.component.TopBarMenuAction
import java.io.File
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

private const val HAR_MIME = "application/json"
private const val HAR_IMPORT_LIMIT = 4 * 1024 * 1024

@Composable
internal fun trafficArchiveActions(
    viewModel: TrafficViewModel,
    batch: Map<String, String>,
    filters: Map<String, String> = emptyMap(),
    single: Boolean = false,
    enabled: Boolean = true,
): List<TopBarMenuAction> {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    var pending by rememberSaveable { mutableStateOf<ArrayList<String>?>(null) }
    var confirm by rememberSaveable { mutableStateOf(false) }
    var busy by remember { mutableStateOf(false) }
    fun failed(error: Exception) {
        if (error is CancellationException) throw error
        val message = if (error is ModuleApiException && error.resultCode in setOf("api.body_too_large", "traffic.har_invalid")) {
            R.string.traffic_har_import_invalid
        } else R.string.traffic_har_failed
        Toast.makeText(context, message, Toast.LENGTH_LONG).show()
    }
    fun selected(): Map<String, String> = pending.orEmpty().chunked(2).associate { it[0] to it[1] }
    val save = rememberLauncherForActivityResult(ActivityResultContracts.CreateDocument(HAR_MIME)) { uri ->
        if (uri != null) {
            val query = selected()
            busy = true
            scope.launch {
                val file = File.createTempFile("capture-", ".har", context.cacheDir)
                try {
                    viewModel.exportHAR(query, file)
                    withContext(Dispatchers.IO) {
                        checkNotNull(context.contentResolver.openOutputStream(uri, "wt")).use { output ->
                            file.inputStream().use { it.copyTo(output) }
                        }
                    }
                    Toast.makeText(context, R.string.traffic_har_saved, Toast.LENGTH_SHORT).show()
                } catch (error: Exception) {
                    failed(error)
                } finally {
                    file.delete()
                    busy = false
                }
            }
        }
    }
    val open = rememberLauncherForActivityResult(ActivityResultContracts.OpenDocument()) { uri ->
        if (uri != null) {
            busy = true
            scope.launch {
                val file = File.createTempFile("import-", ".har", context.cacheDir)
                try {
                    val name = withContext(Dispatchers.IO) {
                        val resolver = context.contentResolver
                        val displayName = resolver.query(uri, arrayOf(OpenableColumns.DISPLAY_NAME), null, null, null)?.use {
                            if (it.moveToFirst()) it.getString(0) else null
                        }
                        checkNotNull(resolver.openInputStream(uri)).use { input ->
                            file.outputStream().use { output ->
                                val buffer = ByteArray(8192)
                                var total = 0
                                while (true) {
                                    val read = input.read(buffer)
                                    if (read < 0) break
                                    total += read
                                    if (total > HAR_IMPORT_LIMIT) throw ModuleApiException("api.body_too_large", "")
                                    output.write(buffer, 0, read)
                                }
                            }
                        }
                        displayName?.take(256) ?: "capture.har"
                    }
                    viewModel.importHAR(name, file)
                } catch (error: Exception) {
                    failed(error)
                } finally {
                    file.delete()
                    busy = false
                }
            }
        }
    }
    AppDialog(show = confirm, title = stringResource(R.string.traffic_har_export),
        summary = stringResource(R.string.traffic_har_warning), onDismissRequest = { confirm = false }) {
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            AppDialogButton(stringResource(R.string.traffic_har_save), {
                confirm = false
                save.launch("capture.har")
            })
            AppDialogButton(stringResource(R.string.traffic_har_share), {
                val query = selected()
                confirm = false
                busy = true
                scope.launch {
                    var file: File? = null
                    try {
                        val dir = File(context.cacheDir, "traffic-shares")
                        val archive = withContext(Dispatchers.IO) {
                            dir.mkdirs()
                            dir.listFiles()?.filter {
                                it.name.endsWith(".har") && System.currentTimeMillis() - it.lastModified() > 86_400_000
                            }?.forEach { it.delete() }
                            File.createTempFile("capture-", ".har", dir)
                        }
                        file = archive
                        viewModel.exportHAR(query, archive)
                        val uri = FileProvider.getUriForFile(context, "${context.packageName}.fileprovider", archive)
                        context.startActivity(Intent.createChooser(Intent(Intent.ACTION_SEND).apply {
                            type = HAR_MIME
                            putExtra(Intent.EXTRA_STREAM, uri)
                            clipData = ClipData.newUri(context.contentResolver, archive.name, uri)
                            addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
                        }, null))
                    } catch (error: Exception) {
                        file?.delete()
                        failed(error)
                    } finally {
                        busy = false
                    }
                }
            })
        }
    }
    fun choose(query: Map<String, String>) {
        pending = ArrayList(query.flatMap { listOf(it.key, it.value) })
        confirm = true
    }
    return buildList {
        add(TopBarMenuAction(stringResource(if (single) R.string.traffic_har_export_record else R.string.traffic_har_export_batch),
            enabled && !busy) { choose(batch) })
        if (!single) {
            add(TopBarMenuAction(stringResource(R.string.traffic_har_export_filtered), enabled && !busy && filters.isNotEmpty()) {
                choose(batch + filters)
            })
            add(TopBarMenuAction(stringResource(R.string.traffic_har_import), !busy) { open.launch(arrayOf("*/*")) })
        }
    }
}
