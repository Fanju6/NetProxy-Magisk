package www.netproxy.web.ui.ui.software

import android.os.Bundle
import android.webkit.WebResourceRequest
import android.webkit.WebResourceResponse
import android.webkit.WebView
import android.webkit.WebViewClient
import androidx.activity.ComponentActivity
import androidx.activity.addCallback
import androidx.fragment.app.FragmentActivity
import androidx.webkit.WebViewAssetLoader
import www.netproxy.web.ui.util.Insets
import www.netproxy.web.ui.util.MonetColorsProvider
import www.netproxy.web.ui.webview.AssetFsPathHandler
import www.netproxy.web.ui.webview.WebViewInterface

/**
 * 软件模式主界面 (非 Root/Module 模式)
 * 
 * 加载 assets 中的预览版 WebUI，不具备 Root 功能
 */
class SoftwareMainActivity : FragmentActivity() {

    private lateinit var webView: WebView
    private lateinit var assetLoader: WebViewAssetLoader

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        
        MonetColorsProvider.updateCss(this)
        
        webView = WebView(this)
        setContentView(webView)
        
        setupWebView()
        
        onBackPressedDispatcher.addCallback(this) {
            if (webView.canGoBack()) {
                webView.goBack()
            } else {
                finish()
            }
        }
    }
    
    private fun setupWebView() {
        webView.settings.apply {
            javaScriptEnabled = true
            domStorageEnabled = true
            allowFileAccess = false // AssetLoader handles this safely
        }
        
        // 配置 AssetLoader 加载 assets/webroot_software 下的内容
        // 使用自定义域名 netproxy.local 并将根路径 / 映射到 AssetLoader
        // 这样可以正确处理 HTML 中的绝对路径 (如 /webui.css)
        val assetHandler = AssetFsPathHandler(
            context = this,
            assetsPath = "webroot_software", 
            insetsSupplier = { Insets.from(window.decorView.rootWindowInsets) },
            onInsetsRequestedListener = null
        )
            
        assetLoader = WebViewAssetLoader.Builder()
            .setDomain("netproxy.local") // 设置自定义域名
            .addPathHandler("/", assetHandler) // 将根路径映射到 assetHandler
            .build()
            
        webView.webViewClient = object : WebViewClient() {
            override fun shouldInterceptRequest(
                view: WebView,
                request: WebResourceRequest
            ): WebResourceResponse? {
                return assetLoader.shouldInterceptRequest(request.url)
            }
        }
        
        // 使用自定义域名加载首页
        webView.loadUrl("https://netproxy.local/index.html")
    }

    /**
     * 处理配置变化，包括主题切换
     * 当系统主题变化时更新莫奈颜色
     */
    override fun onConfigurationChanged(newConfig: android.content.res.Configuration) {
        super.onConfigurationChanged(newConfig)
        webView.settings.textZoom = (newConfig.fontScale * 100).toInt()
        MonetColorsProvider.updateCss(this)
    }
}
