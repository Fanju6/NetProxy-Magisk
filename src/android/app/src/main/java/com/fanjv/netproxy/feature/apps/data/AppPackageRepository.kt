package com.fanjv.netproxy.feature.apps.data

import com.fanjv.netproxy.core.api.ModuleApiClient
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.serialization.Serializable
import kotlinx.serialization.SerialName

@Serializable
internal data class AndroidApp(
    @SerialName("package_name") val packageName: String,
    val label: String,
    val uid: Int,
    val system: Boolean,
    @SerialName("version_code") val versionCode: Long,
    @SerialName("last_update_time") val lastUpdateTime: Long,
    @SerialName("icon_key") val iconKey: String,
)

@Serializable
internal data class AndroidUserApps(val id: Int, val name: String, val revision: String, val apps: List<AndroidApp>)
@Serializable
internal data class AndroidAppsSnapshot(val users: List<AndroidUserApps>)

internal class AppPackageRepository(private val client: ModuleApiClient) {
    private val mutex = Mutex()
    private class Generation {
        val query = Mutex()
        var snapshot: AndroidAppsSnapshot? = null
    }
    private var generation = Generation()

    suspend fun snapshot(): AndroidAppsSnapshot {
        val current = mutex.withLock { generation }
        return current.query.withLock {
            current.snapshot?.let { return@withLock it }
            val result = client.read<AndroidAppsSnapshot>("/android/apps")
            // 旧请求只填充旧代的快照，不得覆盖失效后创建的新缓存。
            current.snapshot = result
            result
        }
    }

    suspend fun icon(key: String): ByteArray = client.image("/android/icons/$key")

    suspend fun invalidatePackageListingCaches() = mutex.withLock {
        generation = Generation()
    }
}
