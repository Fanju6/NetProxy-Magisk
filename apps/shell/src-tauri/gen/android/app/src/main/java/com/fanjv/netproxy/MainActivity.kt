package com.fanjv.netproxy

import android.os.Bundle
import android.content.Intent
import androidx.activity.enableEdgeToEdge

class MainActivity : TauriActivity() {
  override fun onCreate(savedInstanceState: Bundle?) {
    // Shared input stays in the live native session, never in saved instance state.
    // Restoring an old task must not re-import its original external Intent.
    if (savedInstanceState != null && intent?.action == Intent.ACTION_SEND) {
      intent = Intent(this, MainActivity::class.java).setAction(Intent.ACTION_MAIN)
    }
    enableEdgeToEdge()
    super.onCreate(savedInstanceState)
  }
}
