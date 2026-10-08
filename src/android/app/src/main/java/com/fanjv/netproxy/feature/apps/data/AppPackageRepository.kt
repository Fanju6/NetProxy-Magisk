package com.fanjv.netproxy.feature.apps.data

import android.content.Context
import com.fanjv.netproxy.core.command.ShellCommand
import com.fanjv.netproxy.feature.apps.model.UserInfo
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext

/** 通过 root shell 查询应用列表，并通过 PackageManager 解析显示名称。 */
internal class AppPackageRepository(
    private val queryPackages: suspend (List<String>) -> List<String>?,
    private val resolveLabel: (String) -> String,
) {
    constructor(context: Context) : this(
        queryPackages = { arguments ->
            ShellCommand.execute(*arguments.toTypedArray()).let { if (it.isSuccess) it.out else null }
        },
        resolveLabel = { packageName ->
            val manager = context.applicationContext.packageManager
            manager.getApplicationLabel(manager.getApplicationInfo(packageName, 0)).toString()
        },
    )

    private val packageListingCacheMutex = Mutex()
    private var cacheRevision = 0L
    private var cachedUsers: List<UserInfo>? = null
    private val cachedInstalledPackages = HashMap<String, List<String>>()

    suspend fun getUsers(): List<UserInfo> = withContext(Dispatchers.IO) {
        val revision = packageListingCacheMutex.withLock {
            cachedUsers?.let { return@withContext it }
            cacheRevision
        }

        val result = queryPackages(listOf("pm", "list", "users"))
        if (result.isNullOrEmpty()) {
            return@withContext listOf(UserInfo("0", "Owner"))
        }

        val users = ArrayList<UserInfo>()
        for (line in result) {
            currentCoroutineContext().ensureActive()
            val startBracket = line.indexOf('{')
            val endBracket = line.indexOf('}')

            if (startBracket != -1 && endBracket != -1 && startBracket < endBracket) {
                val innerContent = line.substring(startBracket + 1, endBracket)
                val firstColon = innerContent.indexOf(':')
                val lastColon = innerContent.lastIndexOf(':')

                if (firstColon != -1 && firstColon != lastColon) {
                    val id = innerContent.substring(0, firstColon)
                    val name = innerContent.substring(firstColon + 1, lastColon)
                    users.add(UserInfo(id, name))
                }
            }
        }

        if (users.isEmpty()) {
            users.add(UserInfo("0", "Owner"))
        }

        packageListingCacheMutex.withLock {
            currentCoroutineContext().ensureActive()
            if (revision == cacheRevision) cachedUsers = users
        }

        users
    }

    suspend fun getInstalledPackages(userId: String = "0", filter: String = "user"): List<String> =
        withContext(Dispatchers.IO) {
            val cacheKey = "$userId|$filter"
            val revision = packageListingCacheMutex.withLock {
                cachedInstalledPackages[cacheKey]?.let { return@withContext it }
                cacheRevision
            }

            val filterArg = when (filter) {
                "system" -> "-s"
                "user" -> "-3"
                else -> ""
            }
            val command =
                if (filterArg.isEmpty()) {
                    arrayOf("pm", "list", "packages", "--user", userId)
                } else {
                    arrayOf("pm", "list", "packages", "--user", userId, filterArg)
                }
            val result = queryPackages(command.toList()) ?: return@withContext emptyList()

            val packages = ArrayList<String>(result.size)
            for (line in result) {
                currentCoroutineContext().ensureActive()
                if (line.startsWith("package:")) {
                    packages.add(line.substring(8))
                }
            }

            packageListingCacheMutex.withLock {
                currentCoroutineContext().ensureActive()
                if (revision == cacheRevision) cachedInstalledPackages[cacheKey] = packages
            }

            packages
        }

    suspend fun invalidatePackageListingCaches() = packageListingCacheMutex.withLock {
        cacheRevision++
        cachedUsers = null
        cachedInstalledPackages.clear()
    }

    fun label(packageName: String): String = try {
        resolveLabel(packageName)
    } catch (error: Exception) {
        if (error is CancellationException) throw error
        packageName
    }
}
