package www.netproxy.web.ui

import android.annotation.SuppressLint
import android.app.ActivityManager
import android.content.Context
import android.graphics.Bitmap
import android.graphics.Color
import android.os.Build
import android.os.Bundle
import android.view.Gravity
import android.view.ViewGroup
import android.webkit.WebResourceRequest
import android.webkit.WebResourceResponse
import android.webkit.WebView
import android.webkit.WebViewClient
import android.widget.FrameLayout
import android.widget.TextView
import androidx.activity.enableEdgeToEdge
import androidx.appcompat.app.AppCompatActivity
import androidx.core.view.ViewCompat
import androidx.core.view.WindowInsetsCompat
import androidx.lifecycle.lifecycleScope
import androidx.webkit.WebViewAssetLoader
import com.google.android.material.progressindicator.CircularProgressIndicator
import com.topjohnwu.superuser.nio.FileSystemManager
import www.netproxy.web.ui.R
import kotlinx.coroutines.CancellableContinuation
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.suspendCancellableCoroutine
import java.io.ByteArrayInputStream
import java.io.ByteArrayOutputStream
import java.io.File

@SuppressLint("SetJavaScriptEnabled")
class MainActivity : AppCompatActivity(), FileSystemService.Listener {
    
    private val moduleId = "netproxy"
    private val moduleName = "NetProxy"
    private val moduleDir = "/data/adb/modules/$moduleId"
    
    private lateinit var webviewInterface: WebViewInterface
    private var webView: WebView? = null
    private lateinit var container: FrameLayout
    private lateinit var insets: Insets
    private var insetsContinuation: CancellableContinuation<Unit>? = null
    private var isInsetsEnabled = false
    private var enableWebDebugging = false
    
    override fun onCreate(savedInstanceState: Bundle?) {
        // Enable edge to edge
        enableEdgeToEdge()
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            window.isNavigationBarContrastEnforced = false
        }
        
        super.onCreate(savedInstanceState)
        
        MonetColorsProvider.updateCss(this)
        
        // 设置任务描述
        if (moduleName.isNotEmpty()) {
            if (Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU) {
                @Suppress("DEPRECATION")
                setTaskDescription(ActivityManager.TaskDescription(moduleName))
            } else {
                val taskDescription = ActivityManager.TaskDescription.Builder().setLabel(moduleName).build()
                setTaskDescription(taskDescription)
            }
        }
        
        // 显示加载指示器
        showLoading()
        
        // 后台加载应用列表
        lifecycleScope.launch(Dispatchers.IO) {
            if (AppList.getApplist().isEmpty()) {
                AppList.getApps(this@MainActivity)
            }
        }
        
        // 直接检查 Root 权限（参考原版方式）
        FileSystemService.start(this)
    }
    
    private fun showLoading() {
        val progressLayout = FrameLayout(this).apply {
            addView(CircularProgressIndicator(this@MainActivity).apply {
                isIndeterminate = true
                layoutParams = FrameLayout.LayoutParams(
                    ViewGroup.LayoutParams.WRAP_CONTENT,
                    ViewGroup.LayoutParams.WRAP_CONTENT,
                    Gravity.CENTER
                )
            })
        }
        setContentView(progressLayout)
    }
    
    private fun erudaConsole(context: Context): String {
        return context.assets.open("eruda.min.js").bufferedReader().use { it.readText() }
    }
    
    override fun onServiceAvailable(fs: FileSystemManager) {
        // Root 权限检查通过，初始化 WebView 并加载
        lifecycleScope.launch {
            setupWebView()
            loadWebUI(fs)
        }
    }
    
    private suspend fun setupWebView() {
        val prefs = getSharedPreferences("settings", MODE_PRIVATE)
        enableWebDebugging = prefs.getBoolean("enable_web_debugging", BuildConfig.DEBUG)
        WebView.setWebContentsDebuggingEnabled(enableWebDebugging)
        
        insets = Insets(0, 0, 0, 0)
        
        container = FrameLayout(this).apply {
            layoutParams = ViewGroup.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT)
        }
        
        this.webView = WebView(this).apply {
            setBackgroundColor(Color.TRANSPARENT)
            val density = resources.displayMetrics.density
            
            ViewCompat.setOnApplyWindowInsetsListener(container) { view, windowInsets ->
                val inset = windowInsets.getInsets(WindowInsetsCompat.Type.systemBars() or WindowInsetsCompat.Type.displayCutout())
                insets = Insets(
                    top = (inset.top / density).toInt(),
                    bottom = (inset.bottom / density).toInt(),
                    left = (inset.left / density).toInt(),
                    right = (inset.right / density).toInt()
                )
                if (isInsetsEnabled) {
                    view.setPadding(0, 0, 0, 0)
                } else {
                    view.setPadding(inset.left, inset.top, inset.right, inset.bottom)
                }
                insetsContinuation?.resumeWith(Result.success(Unit))
                insetsContinuation = null
                WindowInsetsCompat.CONSUMED
            }
            settings.javaScriptEnabled = true
            settings.domStorageEnabled = true
            settings.allowFileAccess = false
            webviewInterface = WebViewInterface(this@MainActivity, this, moduleDir)
        }
        container.addView(this.webView)
        setContentView(container)
        
        if (insets == Insets(0, 0, 0, 0)) {
            suspendCancellableCoroutine { cont ->
                insetsContinuation = cont
                cont.invokeOnCancellation {
                    insetsContinuation = null
                }
            }
        }
    }
    
    private fun loadWebUI(fs: FileSystemManager) {
        val webRoot = File("$moduleDir/webroot")
        val webViewAssetLoader = WebViewAssetLoader.Builder()
            .setDomain("mui.kernelsu.org")
            .addPathHandler(
                "/",
                RemoteFsPathHandler(
                    this,
                    webRoot,
                    fs,
                    { insets },
                    { enable -> enableInsets(enable) }
                )
            )
            .build()
        val webViewClient = object : WebViewClient() {
            override fun shouldInterceptRequest(
                view: WebView,
                request: WebResourceRequest
            ): WebResourceResponse? {
                val url = request.url
                
                // Handle ksu://icon/[packageName] to serve app icon via WebView
                if (url.scheme.equals("ksu", ignoreCase = true) && url.host.equals("icon", ignoreCase = true)) {
                    val packageName = url.path?.substring(1)
                    if (!packageName.isNullOrEmpty()) {
                        val icon = AppIconUtil.loadAppIconSync(this@MainActivity, packageName, 512)
                        if (icon != null) {
                            val stream = ByteArrayOutputStream()
                            icon.compress(Bitmap.CompressFormat.PNG, 100, stream)
                            val inputStream = ByteArrayInputStream(stream.toByteArray())
                            return WebResourceResponse("image/png", null, inputStream)
                        }
                    }
                }
                
                return webViewAssetLoader.shouldInterceptRequest(url)
            }
            
            override fun onPageFinished(view: WebView?, url: String?) {
                if (enableWebDebugging) {
                    view?.evaluateJavascript(erudaConsole(this@MainActivity), null)
                    view?.evaluateJavascript("eruda.init();", null)
                }
            }
        }
        webView?.apply {
            addJavascriptInterface(webviewInterface, "ksu")
            setWebViewClient(webViewClient)
            loadUrl("https://mui.kernelsu.org/index.html")
        }
    }
    
    override fun onLaunchFailed() {
        // 显示错误信息
        val errorLayout = FrameLayout(this).apply {
            layoutParams = ViewGroup.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                ViewGroup.LayoutParams.MATCH_PARENT
            )
        }
        
        val errorText = TextView(this).apply {
            setText(R.string.please_grant_root)
            textSize = 16f
            gravity = Gravity.CENTER
            layoutParams = FrameLayout.LayoutParams(
                ViewGroup.LayoutParams.WRAP_CONTENT,
                ViewGroup.LayoutParams.WRAP_CONTENT,
                Gravity.CENTER
            )
        }
        
        errorLayout.addView(errorText)
        setContentView(errorLayout)
    }
    
    fun enableInsets(enable: Boolean = true) {
        runOnUiThread {
            if (isInsetsEnabled != enable) {
                isInsetsEnabled = enable
                ViewCompat.requestApplyInsets(container)
            }
        }
    }
    
    override fun onDestroy() {
        super.onDestroy()
        FileSystemService.removeListener(this)
    }
}
