package com.fanjv.netproxy.feature.apps.data

import com.fanjv.netproxy.core.command.NetProxyCtlClient
import com.fanjv.netproxy.core.command.NetProxyCtlOutput
import com.fanjv.netproxy.core.command.NetProxyCtlTransport
import kotlinx.coroutines.runBlocking
import org.junit.Assert.*
import org.junit.Test

class AppPolicyRepositoryTest {
    @Test fun publicAppContractKeepsCommaStringsAndNeverRestarts() = runBlocking {
        val calls = mutableListOf<List<String>>()
        val repository = AppPolicyRepository(NetProxyCtlClient(transport = NetProxyCtlTransport { args, _ ->
            calls += args
            NetProxyCtlOutput(true, listOf("""
                {"schema":1,"ok":true,"code":"app.list","message":"","data":{
                    "enabled":true,"mode":"blacklist",
                    "proxy_apps":"0:com.example.client,10:com.example.client",
                    "bypass_apps":"10:com.example.bypass"
                }}
            """.trimIndent()), emptyList())
        }))
        val config = repository.config()
        assertEquals("0:com.example.client,10:com.example.client", config.proxyApps)
        assertEquals("10:com.example.bypass", config.bypassApps)
        repository.setMode("whitelist")
        repository.add("10:com.example.client")
        repository.remove("0:com.example.client")
        repository.setEnabled(false)
        assertEquals(listOf(
            listOf("app", "list"), listOf("app", "mode", "whitelist"),
            listOf("app", "add", "10:com.example.client"),
            listOf("app", "remove", "0:com.example.client"), listOf("app", "disable")
        ), calls)
    }
}
