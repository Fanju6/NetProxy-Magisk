package com.fanjv.netproxy.feature.traffic.presentation

import android.content.ClipData
import android.content.Intent
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.webkit.MimeTypeMap
import android.widget.VideoView
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.Image
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.core.content.FileProvider
import androidx.lifecycle.compose.LifecycleResumeEffect
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.ui.component.AppDialogButton
import com.fanjv.netproxy.feature.traffic.data.CapturedBody
import java.io.File
import java.util.UUID
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.NonCancellable
import kotlinx.coroutines.awaitCancellation
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import top.yukonga.miuix.kmp.basic.Text

internal fun captureMediaName(filename: String?, extension: String?): String =
    filename.orEmpty().substringAfterLast('/').substringAfterLast('\\')
        .filterNot { it.isISOControl() }.trim().take(120).takeUnless { it.isEmpty() || it == "." || it == ".." }
        ?: "capture.${extension ?: "bin"}"

private data class PreviewImage(val bitmap: Bitmap, val width: Int, val height: Int)

private fun decodePreview(file: File): PreviewImage? {
    val bounds = BitmapFactory.Options().apply { inJustDecodeBounds = true }
    BitmapFactory.decodeFile(file.path, bounds)
    if (bounds.outWidth !in 1..32768 || bounds.outHeight !in 1..32768) return null
    var sample = 1
    while (maxOf(bounds.outWidth, bounds.outHeight) / sample > 2048) sample *= 2
    val options = BitmapFactory.Options().apply { inSampleSize = sample }
    return BitmapFactory.decodeFile(file.path, options)?.let { PreviewImage(it, bounds.outWidth, bounds.outHeight) }
}

@Composable
internal fun TrafficMedia(id: String, side: String, body: CapturedBody, viewModel: TrafficViewModel) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val mime = body.contentType?.substringBefore(';')?.trim()?.lowercase().orEmpty()
    val shareMime = mime.takeIf { it.matches(Regex("[a-z0-9.+_-]+/[a-z0-9.+_-]+")) } ?: "application/octet-stream"
    val size = body.capturedSize ?: 0
    val filename = captureMediaName(body.filename, MimeTypeMap.getSingleton().getExtensionFromMimeType(shareMime))
    val identity = body.contentEncoding.isNullOrBlank() || body.contentEncoding.equals("identity", ignoreCase = true)
    val image = identity && mime.startsWith("image/") && mime != "image/svg+xml"
    val video = identity && mime.startsWith("video/")
    val audio = identity && mime.startsWith("audio/")
    val playable = (video || audio) && !body.truncated && body.contentRange.isNullOrEmpty()
    val available = body.blob && size in 1..(16 * 1024 * 1024)
    var attempt by remember(id, side) { mutableIntStateOf(if (image || playable) 1 else 0) }
    var file by remember(id, side) { mutableStateOf<File?>(null) }
    var decoded by remember(id, side) { mutableStateOf<PreviewImage?>(null) }
    var loadFailed by remember(id, side) { mutableStateOf(false) }
    var exportFailed by remember(id, side) { mutableStateOf(false) }
    val shareTitle = stringResource(R.string.traffic_media_share)
    val save = rememberLauncherForActivityResult(ActivityResultContracts.CreateDocument(shareMime)) { uri ->
        val source = file
        if (uri != null && source != null) scope.launch {
            exportFailed = withContext(Dispatchers.IO) {
                runCatching {
                    requireNotNull(context.contentResolver.openOutputStream(uri, "wt")).use { output ->
                        source.inputStream().use { it.copyTo(output) }
                    }
                }.isFailure
            }
        }
    }
    LaunchedEffect(id, side, size, attempt, available) {
        if (!available || attempt == 0) return@LaunchedEffect
        val directory = File(File(context.cacheDir, "traffic-media"), UUID.randomUUID().toString())
        val destination = File(directory, filename)
        try {
            loadFailed = false
            withContext(Dispatchers.IO) {
                directory.parentFile?.listFiles()?.filter { it.lastModified() < System.currentTimeMillis() - 3_600_000 }
                    ?.forEach { it.deleteRecursively() }
                check(directory.mkdirs())
            }
            viewModel.downloadBody(id, side, destination, size)
            decoded = if (image) withContext(Dispatchers.IO) { decodePreview(destination) } else null
            file = destination
            awaitCancellation()
        } catch (cancelled: CancellationException) {
            throw cancelled
        } catch (_: Exception) {
            loadFailed = true
        } finally {
            withContext(NonCancellable + Dispatchers.IO) { directory.deleteRecursively() }
        }
    }
    InspectCard {
        Text(filename)
        Text(listOfNotNull(mime.takeIf(String::isNotBlank), body.contentEncoding, body.contentRange).joinToString(" · "))
        Text(stringResource(R.string.traffic_media_sizes, size.toLong(), body.size ?: 0L))
        if (body.truncated) Text(stringResource(R.string.traffic_media_truncated))
        if (!available) {
            Text(stringResource(R.string.traffic_media_unavailable))
        } else if (file == null) {
            if (attempt == 0 || loadFailed) AppDialogButton(stringResource(R.string.traffic_media_load), { attempt++ })
            if (loadFailed) Text(stringResource(R.string.traffic_media_failed))
            else if (attempt > 0) Text(stringResource(R.string.traffic_media_loading))
        } else {
            decoded?.let {
                Text("${it.width} × ${it.height}")
                Image(it.bitmap.asImageBitmap(), filename, Modifier.fillMaxWidth().heightIn(max = 360.dp))
            }
            if (image && decoded == null) Text(stringResource(R.string.traffic_media_preview_unavailable))
            if (playable) file?.let { CapturedPlayer(it, video) }
            if ((audio || video) && !playable) Text(stringResource(R.string.traffic_media_preview_unavailable))
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                AppDialogButton(stringResource(R.string.traffic_media_save), { save.launch(filename) }, Modifier.weight(1f))
                AppDialogButton(shareTitle, {
                    val source = file ?: return@AppDialogButton
                    scope.launch {
                        val uri = withContext(Dispatchers.IO) {
                            runCatching {
                                val root = File(context.cacheDir, "traffic-shares").apply { mkdirs() }
                                root.listFiles().orEmpty().sortedByDescending(File::lastModified)
                                    .drop(3).forEach { it.deleteRecursively() }
                                val copy = File(File(root, UUID.randomUUID().toString()).apply { mkdirs() }, filename)
                                source.copyTo(copy)
                                FileProvider.getUriForFile(context, "${context.packageName}.fileprovider", copy)
                            }.getOrNull()
                        }
                        exportFailed = uri == null || runCatching {
                            context.startActivity(Intent.createChooser(Intent(Intent.ACTION_SEND).apply {
                                type = shareMime
                                putExtra(Intent.EXTRA_STREAM, uri)
                                clipData = ClipData.newRawUri(filename, uri)
                                addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
                            }, shareTitle))
                        }.isFailure
                    }
                }, Modifier.weight(1f))
            }
            if (exportFailed) Text(stringResource(R.string.traffic_media_failed))
        }
    }
}

@Composable
private fun CapturedPlayer(file: File, video: Boolean) {
    var player by remember(file) { mutableStateOf<VideoView?>(null) }
    var ready by remember(file) { mutableStateOf(false) }
    var playing by remember(file) { mutableStateOf(false) }
    var failed by remember(file) { mutableStateOf(false) }
    LifecycleResumeEffect(file) {
        onPauseOrDispose {
            player?.pause()
            playing = false
        }
    }
    AndroidView(
        modifier = Modifier.fillMaxWidth().height(if (video) 208.dp else 1.dp),
        factory = { context ->
            VideoView(context).apply {
                player = this
                setOnPreparedListener {
                    ready = true
                    if (video) seekTo(1)
                }
                setOnCompletionListener { playing = false }
                setOnErrorListener { _, _, _ ->
                    ready = false
                    failed = true
                    playing = false
                    true
                }
                setVideoPath(file.absolutePath)
            }
        },
        onRelease = { it.stopPlayback() },
    )
    if (failed) Text(stringResource(R.string.traffic_media_preview_unavailable))
    else AppDialogButton(
        stringResource(if (playing) R.string.traffic_media_pause else R.string.traffic_media_play),
        onClick = {
            if (playing) player?.pause() else player?.start()
            playing = !playing
        },
        enabled = ready,
    )
}
