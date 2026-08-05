package com.fanjv.netproxy.feature.apps.data

import android.content.Context
import com.fanjv.netproxy.core.command.ShellCommand
import com.fanjv.netproxy.feature.apps.model.UserInfo
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext

/** 通过 root shell 查询应用列表，并通过 PackageManager 解析显示名称。 */
internal class AppPackageRepository(context: Context) {
    private val packageManager = context.applicationContext.packageManager
    private val packageListingCacheMutex = Mutex()
    private var cachedUsers: List<UserInfo>? = null
    private val cachedInstalledPackages = HashMap<String, List<String>>()

    suspend fun getUsers(): List<UserInfo> = withContext(Dispatchers.IO) {
        packageListingCacheMutex.withLock {
            cachedUsers?.let { return@withContext it }
        }

        val result = ShellCommand.exec("pm", "list", "users")
        if (!result.isSuccess || result.out.isEmpty()) {
            return@withContext listOf(UserInfo("0", "Owner"))
        }

        val users = ArrayList<UserInfo>()
        for (line in result.out) {
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
            cachedUsers = users
        }

        users
    }

    suspend fun getInstalledPackages(userId: String = "0", filter: String = "user"): List<String> =
        withContext(Dispatchers.IO) {
            val cacheKey = "$userId|$filter"
            packageListingCacheMutex.withLock {
                cachedInstalledPackages[cacheKey]?.let { return@withContext it }
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
            val result = ShellCommand.exec(*command)
            if (!result.isSuccess) return@withContext emptyList()

            val packages = ArrayList<String>(result.out.size)
            for (line in result.out) {
                if (line.startsWith("package:")) {
                    packages.add(line.substring(8))
                }
            }

            packageListingCacheMutex.withLock {
                cachedInstalledPackages[cacheKey] = packages
            }

            packages
        }

    suspend fun invalidatePackageListingCaches() = packageListingCacheMutex.withLock {
        cachedUsers = null
        cachedInstalledPackages.clear()
    }

    fun label(packageName: String): String = runCatching {
        val info = packageManager.getApplicationInfo(packageName, 0)
        packageManager.getApplicationLabel(info).toString()
    }.getOrDefault(packageName)
}
