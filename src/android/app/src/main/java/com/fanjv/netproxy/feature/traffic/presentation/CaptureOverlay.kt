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
import kotlinx.coroutines.coroutineScope
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
    private var paused: TextView? = null
    private var rows = emptyList<TextView>()
    private var positionX = preferences.getInt("x", dp(12))
    private var positionY = preferences.getInt("y", dp(72))
    private var minimized = preferences.getBoolean("minimized", false)
    private var rightEdge = preferences.getBoolean("right_edge", false)
    private var recordCount = 0
    private var records = emptyList<CaptureRecord>()
    private var pauseIDs = emptyList<String>()
    private var highlight: Job? = null

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
                coroutineScope {
                    val changes = launch {
                        repository.observePauses().collect { result ->
                            val items = result.getOrNull().orEmpty()
                            val ids = items.map { it.id }
                            val added = ids.any { it !in pauseIDs }
                            pauseIDs = ids
                            if (ids.isNotEmpty() && panel == null) show()
                            updatePause()
                            if (minimized && added) flashTab()
                        }
                    }
                    try {
                        while (enabled && Settings.canDrawOverlays(context) && power.isInteractive && !keyguard.isKeyguardLocked) {
                            val preview = repository.preview()
                            if (!preview.running) break
                            render(preview.count, preview.entries)
                            delay(2_000)
                        }
                    } finally {
                        changes.cancel()
                    }
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
        recordCount = 0
        records = emptyList()
        pauseIDs = emptyList()
        hide()
    }

    private fun hide() {
        highlight?.cancel()
        panel?.let { runCatching { window.removeViewImmediate(it) } }
        panel = null
        header = null
        paused = null
        rows = emptyList()
    }

    private fun render(count: Int, records: List<CaptureRecord>) {
        recordCount = count
        this.records = records
        if (panel == null) show()
        updatePanel()
    }

    private fun updatePanel() {
        header?.text = if (minimized) recordCount.toString() else context.getString(R.string.traffic_record_count, recordCount)
        if (minimized) {
            header?.contentDescription = context.getString(R.string.traffic_overlay_expand, recordCount)
        }
        updatePause()
        rows.forEachIndexed { index, row ->
            val record = records.getOrNull(index)
            row.visibility = if (record == null) View.GONE else View.VISIBLE
            row.text = record?.let { "${it.method.take(12)}  ${it.host.take(80)}  ${it.status.takeIf { code -> code > 0 } ?: "—"}" }
        }
    }

    private fun updatePause() {
        val count = pauseIDs.size
        val label = context.resources.getQuantityString(R.plurals.traffic_breakpoint_count, count, count)
        paused?.visibility = if (count > 0) View.VISIBLE else View.GONE
        paused?.text = if (minimized) "! $count" else label
        paused?.contentDescription = label
    }

    private fun flashTab() {
        highlight?.cancel()
        val background = panel?.background as? GradientDrawable ?: return
        background.setColor(Color.rgb(67, 108, 116))
        highlight = scope.launch {
            delay(1_200)
            background.setColor(Color.rgb(32, 44, 48))
        }
    }

    private fun toggleMinimized() {
        val width = context.resources.displayMetrics.widthPixels
        if (!minimized) {
            rightEdge = positionX + (panel?.width ?: dp(248)) / 2 >= width / 2
        } else {
            positionX = if (rightEdge) (width - dp(248)).coerceAtLeast(0) else 0
        }
        minimized = !minimized
        persistPosition()
        hide()
        show()
    }

    private fun persistPosition() {
        preferences.edit().putBoolean("minimized", minimized).putBoolean("right_edge", rightEdge)
            .putInt("x", positionX).putInt("y", positionY).apply()
    }

    @SuppressLint("RtlHardcoded")
    private fun show() {
        val content = LinearLayout(context).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(dp(if (minimized) 0 else 12), dp(4), dp(if (minimized) 0 else 8), dp(8))
            background = GradientDrawable().apply {
                setColor(Color.rgb(32, 44, 48))
                cornerRadius = dp(16).toFloat()
            }
            elevation = dp(8).toFloat()
        }
        val metrics = context.resources.displayMetrics
        val width = dp(if (minimized) 52 else 248).coerceAtMost(metrics.widthPixels)
        val params = WindowManager.LayoutParams(
            width,
            WindowManager.LayoutParams.WRAP_CONTENT,
            WindowManager.LayoutParams.TYPE_APPLICATION_OVERLAY,
            WindowManager.LayoutParams.FLAG_NOT_FOCUSABLE or WindowManager.LayoutParams.FLAG_NOT_TOUCH_MODAL,
            PixelFormat.TRANSLUCENT,
        ).apply {
            gravity = Gravity.TOP or Gravity.LEFT
            x = if (minimized) (if (rightEdge) metrics.widthPixels - width else 0)
                else positionX.coerceIn(0, metrics.widthPixels - width)
            y = positionY.coerceIn(0, (metrics.heightPixels - dp(if (minimized) 140 else 224)).coerceAtLeast(0))
        }
        val title = text(14f).apply {
            minHeight = dp(44)
            if (minimized) gravity = Gravity.CENTER
            setOnClickListener { if (minimized) toggleMinimized() else openRecords() }
            enableDrag(content, params)
        }
        if (minimized) {
            content.addView(title, LinearLayout.LayoutParams(-1, dp(44)))
            paused = text(12f).apply {
                gravity = Gravity.CENTER
                setTextColor(Color.rgb(255, 194, 100))
                setOnClickListener { toggleMinimized() }
                enableDrag(content, params)
                content.addView(this, LinearLayout.LayoutParams(-1, dp(32)))
            }
            window.addView(content, params)
            panel = content
            header = title
            updatePanel()
            return
        }
        val minimize = text(20f).apply {
            text = "−"
            gravity = Gravity.CENTER
            contentDescription = context.getString(R.string.traffic_overlay_minimize)
            setOnClickListener { toggleMinimized() }
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
            addView(minimize, LinearLayout.LayoutParams(dp(44), dp(44)))
            addView(close, LinearLayout.LayoutParams(dp(44), dp(44)))
        })
        paused = text(14f).apply {
            minHeight = dp(48)
            visibility = View.GONE
            setOnClickListener { openRecords(OpenPaused, pauseIDs.singleOrNull()) }
            content.addView(this)
        }
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
        updatePanel()
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
                MotionEvent.ACTION_UP, MotionEvent.ACTION_CANCEL -> {
                    if (moving) {
                        if (minimized) {
                            val width = context.resources.displayMetrics.widthPixels
                            rightEdge = params.x + content.width / 2 >= width / 2
                            positionX = if (rightEdge) (width - content.width).coerceAtLeast(0) else 0
                            params.x = positionX
                            window.updateViewLayout(content, params)
                        }
                        persistPosition()
                    } else if (event.actionMasked == MotionEvent.ACTION_UP) {
                        view.performClick()
                    }
                }
            }
            true
        }
    }

    private fun openRecords(destination: String = OpenRecords, pauseId: String? = null) {
        leaveBackground()
        val intent = Intent(context, MainActivity::class.java).apply {
            action = destination
            if (pauseId != null) putExtra(PauseId, pauseId)
            addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP or Intent.FLAG_ACTIVITY_SINGLE_TOP)
        }
        context.startActivity(intent)
    }

    private fun dp(value: Int) = (context.resources.displayMetrics.density * value).roundToInt()

    companion object {
        const val OpenRecords = "com.fanjv.netproxy.OPEN_CAPTURE_RECORDS"
        const val OpenPaused = "com.fanjv.netproxy.OPEN_CAPTURE_PAUSED"
        const val PauseId = "com.fanjv.netproxy.CAPTURE_PAUSE_ID"
    }
}
