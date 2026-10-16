package com.fanjv.netproxy.bridge

import android.webkit.WebView
import androidx.appcompat.app.AppCompatActivity
import app.tauri.annotation.Command
import app.tauri.annotation.TauriPlugin
import app.tauri.plugin.Invoke
import org.junit.Assert.*
import org.junit.Test

class ModuleApiPluginContractTest {
    @Test fun pluginHasTheOfficialNameAndSixCommands() {
        val plugin = ModuleApiPlugin::class.java
        assertEquals("com.fanjv.netproxy.bridge.ModuleApiPlugin", plugin.name)
        assertNotNull(plugin.getAnnotation(TauriPlugin::class.java))
        val commands = plugin.declaredMethods.filter { it.getAnnotation(Command::class.java) != null }
        assertEquals(setOf("read", "cancel", "openEvents", "nextEvent", "closeEvents", "visibility"), commands.map { it.name }.toSet())
        commands.forEach { assertArrayEquals(arrayOf(Invoke::class.java), it.parameterTypes) }
    }

    @Test fun listenersRemainOfficialInheritedCommands() {
        for (name in listOf("registerListener", "removeListener")) {
            val method = ModuleApiPlugin::class.java.getMethod(name, Invoke::class.java)
            assertEquals("app.tauri.plugin.Plugin", method.declaringClass.name)
            assertNotNull(method.getAnnotation(Command::class.java))
        }
    }

    @Test fun lifecycleUsesOnlyCurrentActivityAwareCallbacksAndNeverOwnsWebViewDestruction() {
        val plugin = ModuleApiPlugin::class.java
        assertNotNull(plugin.getDeclaredMethod("load", WebView::class.java))
        for (name in listOf("onPause", "onResume", "onDestroy")) {
            assertNotNull(plugin.getDeclaredMethod(name, AppCompatActivity::class.java))
            assertFalse(plugin.declaredMethods.any { it.name == name && it.parameterCount == 0 })
        }
        assertFalse(plugin.declaredFields.any { WebView::class.java.isAssignableFrom(it.type) })
    }

    @Test fun readArgumentsUseResourceNotArbitraryPathOrMethod() {
        val fields = ReadArgs::class.java.declaredFields.map { it.name }.toSet()
        assertEquals(setOf("requestId", "resource"), fields)
    }
}
