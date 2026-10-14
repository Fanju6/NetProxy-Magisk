package com.fanjv.netproxy.feature.kernel.presentation

import com.fanjv.netproxy.core.api.*

import com.fanjv.netproxy.feature.settings.model.ConfigSnapshot
import com.fanjv.netproxy.feature.settings.model.ManagedConfigDocument
import kotlinx.serialization.json.Json
import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Test

class ConfigDocumentContractTest {
    private val json = Json { ignoreUnknownKeys = true }

    @Test
    fun sectionDocumentsAndSnapshotsKeepTheirIdentity() {
        val document = json.decodeFromString<ManagedConfigDocument>(
            """{"id":"singbox/dns","filename":"dns","category":"config","editable":true,"section":"dns"}"""
        )
        assertEquals("dns", document.section)
        assertEquals("singbox/dns", document.id)
        val snapshot = json.decodeFromString<ConfigSnapshot>(
            """{"target":"singbox/dns","content":"{\"dns\":{}}","revision":"snapshot-1"}"""
        )
        assertEquals("{\"dns\":{}}", snapshot.content)
        assertEquals("snapshot-1", snapshot.revision)
    }

    @Test
    fun conflictsRemainActionableErrors() {
        val error = assertThrows(ModuleApiException::class.java) {
            ModuleApiCodec(json).decode(409,
                """{"schema":1,"ok":false,"code":"config.conflict","message":"配置已被修改，请重新加载后再保存"}""")
        }
        assertEquals("config.conflict", error.resultCode)
        assertEquals("配置已被修改，请重新加载后再保存", error.message)
    }
}
