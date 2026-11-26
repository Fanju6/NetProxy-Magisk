package com.fanjv.netproxy.feature.traffic.presentation

import android.annotation.SuppressLint
import android.app.KeyguardManager
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.graphics.Color
import android.graphics.PixelFormat
import android.graphics.drawable.GradientDrawable
import android.os.PowerManager
import android.provider.Settings
import android.text.TextUtils
import android.view.Gravity
import android.view.MotionEvent
import android.view.View
import android.view.ViewConfiguration
import android.view.WindowManager
import android.widget.LinearLayout
import android.widget.TextView
import androidx.core.content.ContextCompat
import com.fanjv.netproxy.MainActivity
import com.fanjv.netproxy.R
import com.fanjv.netproxy.feature.traffic.data.CaptureRecord
import com.fanjv.netproxy.feature.traffic.data.TrafficRepository
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlin.math.abs
import kotlin.math.roundToInt

internal class CaptureOverlay(private val context: Context, private val repository: TrafficRepository) {
    private val preferences = context.getSharedPreferences("capture_overlay", Context.MODE_PRIVATE)
    private val window = context.getSystemService(WindowManager::class.java)
    private val power = context.getSystemService(PowerManager::class.java)
    private val keyguard = context.getSystemService(KeyguardManager::class.java)
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)
    private var refresh: Job? = null
    private var generation = 0
    private var background = false
    private var panel: LinearLayout? = null
    private var header: TextView? = null
    private var rows = emptyList<TextView>()
    private var positionX = dp(12)
    private var positionY = dp(72)

    private val screen = object : BroadcastReceiver() {
        override fun onReceive(context: Context, intent: Intent) {
            if (intent.action == Intent.ACTION_SCREEN_OFF) stopPreview() else startPreview()
        }
    }

    var enabled: Boolean
        get() = preferences.getBoolean("enabled", true)
        set(value) { preferences.edit().putBoolean("enabled", value).apply() }

    fun claimPermissionHint(): Boolean {
        if (preferences.getBoolean("asked", false)) return false
        preferences.edit().putBoolean("asked", true).apply()
        return true
    }

    fun enterBackground() {
        if (background || !enabled || !Settings.canDrawOverlays(context)) return
        background = true
        ContextCompat.registerReceiver(context, screen, IntentFilter().apply {
            addAction(Intent.ACTION_SCREEN_OFF)
            addAction(Intent.ACTION_SCREEN_ON)
            addAction(Intent.ACTION_USER_PRESENT)
        }, ContextCompat.RECEIVER_NOT_EXPORTED)
        startPreview()
    }

    fun leaveBackground() {
        if (background) context.unregisterReceiver(screen)
        background = false
        stopPreview()
    }

    private fun startPreview() {
        if (!background || refresh?.isActive == true || !power.isInteractive || keyguard.isKeyguardLocked) return
        val currentGeneration = ++generation
        refresh = scope.launch {
            try {
                while (enabled && Settings.canDrawOverlays(context) && power.isInteractive && !keyguard.isKeyguardLocked) {
                    val preview = repository.preview()
                    if (!preview.running) break
                    render(preview.count, preview.entries)
                    delay(2_000)
                }
            } catch (e: CancellationException) {
                throw e
            } catch (_: Exception) {
                // Do not display stale traffic after losing the module connection or overlay permission.
            } finally {
                if (generation == currentGeneration) hide()
            }
        }
    }

    private fun stopPreview() {
        generation++
        refresh?.cancel()
        refresh = null
        hide()
    }

    private fun hide() {
        panel?.let { runCatching { window.removeViewImmediate(it) } }
        panel = null
        header = null
        rows = emptyList()
    }

    private fun render(count: Int, records: List<CaptureRecord>) {
        if (panel == null) show()
        header?.text = context.getString(R.string.traffic_record_count, count)
        rows.forEachIndexed { index, row ->
            val record = records.getOrNull(index)
            row.visibility = if (record == null) View.GONE else View.VISIBLE
            row.text = record?.let { "${it.method.take(12)}  ${it.host.take(80)}  ${it.status.takeIf { code -> code > 0 } ?: "—"}" }
        }
    }

    @SuppressLint("RtlHardcoded")
    private fun show() {
        val content = LinearLayout(context).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(dp(12), dp(4), dp(8), dp(8))
            background = GradientDrawable().apply {
                setColor(Color.rgb(32, 44, 48))
                cornerRadius = dp(16).toFloat()
            }
            elevation = dp(8).toFloat()
        }
        val params = WindowManager.LayoutParams(
            dp(248).coerceAtMost(context.resources.displayMetrics.widthPixels),
            WindowManager.LayoutParams.WRAP_CONTENT,
            WindowManager.LayoutParams.TYPE_APPLICATION_OVERLAY,
            WindowManager.LayoutParams.FLAG_NOT_FOCUSABLE or WindowManager.LayoutParams.FLAG_NOT_TOUCH_MODAL,
            PixelFormat.TRANSLUCENT,
        ).apply {
            gravity = Gravity.TOP or Gravity.LEFT
            x = positionX
            y = positionY
        }
        val title = text(14f).apply {
            minHeight = dp(44)
            setOnClickListener { openRecords() }
            enableDrag(content, params)
        }
        val close = text(20f).apply {
            text = "×"
            gravity = Gravity.CENTER
            contentDescription = context.getString(R.string.traffic_overlay_close)
            setOnClickListener { leaveBackground() }
        }
        content.addView(LinearLayout(context).apply {
            gravity = Gravity.CENTER_VERTICAL
            addView(title, LinearLayout.LayoutParams(0, dp(44), 1f))
            addView(close, LinearLayout.LayoutParams(dp(44), dp(44)))
        })
        val previewRows = List(3) {
            text(12f).apply {
                visibility = View.GONE
                setOnClickListener { openRecords() }
                content.addView(this)
            }
        }
        window.addView(content, params)
        panel = content
        header = title
        rows = previewRows
    }

    private fun text(size: Float) = TextView(context).apply {
        textSize = size
        setTextColor(Color.WHITE)
        gravity = Gravity.CENTER_VERTICAL
        maxLines = 1
        ellipsize = TextUtils.TruncateAt.END
    }

    @SuppressLint("ClickableViewAccessibility")
    private fun TextView.enableDrag(content: View, params: WindowManager.LayoutParams) {
        val slop = ViewConfiguration.get(context).scaledTouchSlop
        var initialX = 0f
        var initialY = 0f
        var startX = 0
        var startY = 0
        var moving = false
        setOnTouchListener { view, event ->
            when (event.actionMasked) {
                MotionEvent.ACTION_DOWN -> {
                    initialX = event.rawX
                    initialY = event.rawY
                    startX = params.x
                    startY = params.y
                    moving = false
                }
                MotionEvent.ACTION_MOVE -> {
                    val dx = event.rawX - initialX
                    val dy = event.rawY - initialY
                    if (abs(dx) > slop || abs(dy) > slop) moving = true
                    if (moving) {
                        val metrics = context.resources.displayMetrics
                        positionX = (startX + dx.roundToInt()).coerceIn(0, (metrics.widthPixels - content.width).coerceAtLeast(0))
                        positionY = (startY + dy.roundToInt()).coerceIn(0, (metrics.heightPixels - content.height - dp(48)).coerceAtLeast(0))
                        params.x = positionX
                        params.y = positionY
                        window.updateViewLayout(content, params)
                    }
                }
                MotionEvent.ACTION_UP -> if (!moving) view.performClick()
            }
            true
        }
    }

    private fun openRecords() {
        leaveBackground()
        val intent = Intent(context, MainActivity::class.java).apply {
            action = OpenRecords
            addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP or Intent.FLAG_ACTIVITY_SINGLE_TOP)
        }
        context.startActivity(intent)
    }

    private fun dp(value: Int) = (context.resources.displayMetrics.density * value).roundToInt()

    companion object {
        const val OpenRecords = "com.fanjv.netproxy.OPEN_CAPTURE_RECORDS"
    }
}
