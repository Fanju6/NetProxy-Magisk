package www.netproxy.web.ui.webview

import android.content.Context
import android.content.res.AssetManager
import android.webkit.WebResourceResponse
import androidx.webkit.WebViewAssetLoader
import www.netproxy.web.ui.util.Insets
import www.netproxy.web.ui.util.MonetColorsProvider
import java.io.ByteArrayInputStream
import java.io.IOException
import java.io.InputStream
import java.nio.charset.StandardCharsets

/**
 * Handler class to open files from assets
 */
class AssetFsPathHandler(
    private val context: Context,
    private val assetsPath: String = "webroot_software",
    private val insetsSupplier: () -> Insets,
    private val onInsetsRequestedListener: ((Boolean) -> Unit)?
) : WebViewAssetLoader.PathHandler {

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

        try {
            // Remove matching path prefix if present (though AssetLoader usually strips the path prefix mapped to this handler)
            // But here we are manually mapping file path.
            // Actually WebViewAssetLoader.AssetsPathHandler does this automatically if we use it.
            // But we need custom handling for insets/colors, so we implement PathHandler manually.
            
            val assetPath = if (path.startsWith("/")) path.substring(1) else path
            val fullPath = "$assetsPath/$assetPath"
            
            val inputStream = context.assets.open(fullPath)
            val mimeType = MimeUtil.getMimeFromFileName(fullPath) ?: "text/plain"
            return WebResourceResponse(mimeType, null, inputStream)
        } catch (e: IOException) {
            // File not found or other IO error
        }
        
        return WebResourceResponse(null, null, null)
    }
}
