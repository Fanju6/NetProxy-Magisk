package www.netproxy.web.ui.webview

import android.content.Context
import android.util.Log
import android.webkit.WebResourceResponse
import androidx.webkit.WebViewAssetLoader
import com.topjohnwu.superuser.nio.FileSystemManager
import www.netproxy.web.ui.util.Insets
import www.netproxy.web.ui.util.MonetColorsProvider
import java.io.ByteArrayInputStream
import java.io.File
import java.io.IOException
import java.io.InputStream
import java.nio.charset.StandardCharsets
import java.util.zip.GZIPInputStream

/**
 * WebView 远程文件系统路径处理器
 * 
 * 通过 Root 权限访问模块目录下的 WebUI 资源文件，
 * 同时提供 insets.css 和 colors.css 等动态生成的内容
 * 
 * @param context 上下文
 * @param directory WebUI 资源目录
 * @param fs 文件系统管理器
 * @param insetsSupplier 窗口边距提供器
 * @param onInsetsRequestedListener 边距请求监听器
 */
class RemoteFsPathHandler(
    private val context: Context,
    directory: File,
    private val fs: FileSystemManager,
    private val insetsSupplier: () -> Insets,
    private val onInsetsRequestedListener: ((Boolean) -> Unit)?
) : WebViewAssetLoader.PathHandler {
    
    companion object {
        private const val TAG = "FsServicePathHandler"
        
        /** 默认 MIME 类型 */
        const val DEFAULT_MIME_TYPE = "text/plain"
        
        /** 禁止访问的目录 */
        private val FORBIDDEN_DATA_DIRS = arrayOf("/data/data", "/data/system")
        
        /**
         * 获取文件的规范目录路径（确保以 / 结尾）
         */
        @Throws(IOException::class)
        fun getCanonicalDirPath(file: File): String {
            var canonicalPath = file.canonicalPath
            if (!canonicalPath.endsWith("/")) {
                canonicalPath += "/"
            }
            return canonicalPath
        }
        
        /**
         * 获取子文件的规范路径（仅当在父目录内时返回）
         */
        @Throws(IOException::class)
        fun getCanonicalFileIfChild(parent: File, child: String): File? {
            val parentCanonicalPath = getCanonicalDirPath(parent)
            val childCanonicalPath = File(parent, child).canonicalPath
            return if (childCanonicalPath.startsWith(parentCanonicalPath)) {
                File(childCanonicalPath)
            } else {
                null
            }
        }
        
        /**
         * 打开文件，自动处理 .svgz 压缩格式
         */
        @Throws(IOException::class)
        fun openFile(file: File, fs: FileSystemManager): InputStream {
            val stream = fs.getFile(file.absolutePath).newInputStream()
            return handleSvgzStream(file.path, stream)
        }
        
        /**
         * 处理 SVGZ 压缩流
         */
        @Throws(IOException::class)
        private fun handleSvgzStream(path: String, stream: InputStream): InputStream {
            return if (path.endsWith(".svgz")) GZIPInputStream(stream) else stream
        }
        
        /**
         * 猜测文件的 MIME 类型
         */
        fun guessMimeType(filePath: String): String {
            return MimeUtil.getMimeFromFileName(filePath) ?: DEFAULT_MIME_TYPE
        }
    }
    
    private val directory: File
    
    init {
        try {
            this.directory = File(getCanonicalDirPath(directory))
            if (!isAllowedInternalStorageDir()) {
                throw IllegalArgumentException(
                    "The given directory \"$directory\" doesn't exist under an allowed app internal storage directory"
                )
            }
        } catch (e: IOException) {
            throw IllegalArgumentException(
                "Failed to resolve the canonical path for the given directory: ${directory.path}", e
            )
        }
    }
    
    @Throws(IOException::class)
    private fun isAllowedInternalStorageDir(): Boolean {
        val dir = getCanonicalDirPath(directory)
        return FORBIDDEN_DATA_DIRS.none { dir.startsWith(it) }
    }
    
    override fun handle(path: String): WebResourceResponse {
        // 处理内置 CSS 请求
        if (path == "internal/insets.css") {
            onInsetsRequestedListener?.invoke(true)
            val css = insetsSupplier().css
            return WebResourceResponse(
                "text/css",
                "utf-8",
                ByteArrayInputStream(css.toByteArray(StandardCharsets.UTF_8))
            )
        }
        
        if (path == "internal/colors.css") {
            val enableMonet = context.getSharedPreferences("settings", Context.MODE_PRIVATE)
                .getBoolean("enable_monet", true)
            val css = if (enableMonet) MonetColorsProvider.getColorsCss() else ""
            return WebResourceResponse(
                "text/css",
                "utf-8",
                ByteArrayInputStream(css.toByteArray(StandardCharsets.UTF_8))
            )
        }
        
        // 处理文件请求
        try {
            val file = getCanonicalFileIfChild(directory, path)
            if (file != null) {
                val inputStream = openFile(file, fs)
                val mimeType = guessMimeType(path)
                return WebResourceResponse(mimeType, null, inputStream)
            } else {
                Log.e(TAG, "The requested file: $path is outside the mounted directory: $directory")
            }
        } catch (e: IOException) {
            Log.e(TAG, "Error opening the requested path: $path", e)
        }
        
        return WebResourceResponse(null, null, null)
    }
}
