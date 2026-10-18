package com.fanjv.netproxy.bridge

import android.webkit.WebView
import androidx.appcompat.app.AppCompatActivity
import app.tauri.annotation.Command
import app.tauri.annotation.TauriPlugin
import app.tauri.plugin.Invoke
import org.junit.Assert.*
import org.junit.Test

class ModuleApiPluginContractTest {
    @Test fun pluginHasTheOfficialNameAndRestrictedCommands() {
        val plugin = ModuleApiPlugin::class.java
        assertEquals("com.fanjv.netproxy.bridge.ModuleApiPlugin", plugin.name)
        assertNotNull(plugin.getAnnotation(TauriPlugin::class.java))
        val commands = plugin.declaredMethods.filter { it.getAnnotation(Command::class.java) != null }
        assertEquals(setOf("read", "subscriptionRead", "subscriptionAction", "subscriptionWrite", "management", "serviceAction", "policySelect", "changeMode", "policyDelay", "cancel", "openEvents", "nextEvent", "closeEvents", "visibility", "setAppearance", "registerListener", "removeListener"), commands.map { it.name }.toSet())
        commands.forEach { assertArrayEquals(arrayOf(Invoke::class.java), it.parameterTypes) }
    }

    @Test fun listenersKeepTheOfficialSignaturesAndCommandNames() {
        for (name in listOf("registerListener", "removeListener")) {
            val method = ModuleApiPlugin::class.java.getMethod(name, Invoke::class.java)
            assertEquals(ModuleApiPlugin::class.java, method.declaringClass)
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

    @Test fun serviceArgumentsCannotSupplyURLMethodBodyOrCredentials() {
        assertEquals(setOf("requestId", "action"), ServiceActionArgs::class.java.declaredFields.map { it.name }.toSet())
        assertEquals("", ServiceActionArgs().requestId)
        assertEquals("", ServiceActionArgs().action)
    }

    @Test fun policyAndModeArgumentsCannotSupplyURLMethodBodyOrCredentials() {
        assertEquals(setOf("requestId", "groupTag", "memberTag"), PolicyGroups.Select::class.java.declaredFields.filterNot { it.isSynthetic || it.name == "Companion" }.map { it.name }.toSet())
        assertEquals(setOf("requestId", "groupTag"), PolicyGroups.Delay::class.java.declaredFields.filterNot { it.isSynthetic || it.name == "Companion" }.map { it.name }.toSet())
        assertEquals(setOf("requestId", "mode"), ChangeModeArgs::class.java.declaredFields.map { it.name }.toSet())
        assertNull(ChangeModeArgs().mode)
    }

    @Test fun subscriptionArgumentsCannotSupplyPrivateQueryOrMutation() {
        assertEquals(setOf("requestId", "groupId", "resource"), SubscriptionReadArgs::class.java.declaredFields.map { it.name }.toSet())
    }

    @Test fun subscriptionActionsHaveOnlyFixedIdentityAndActionFields() {
        assertEquals(setOf("requestId", "groupId", "action"),
            SubscriptionActions.Request::class.java.declaredFields.filterNot { it.isSynthetic || it.name == "Companion" }.map { it.name }.toSet())
    }
}
