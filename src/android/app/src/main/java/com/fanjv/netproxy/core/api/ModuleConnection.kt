package com.fanjv.netproxy.core.api

import android.content.Context
import android.util.AtomicFile
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import java.io.File
import java.io.FileNotFoundException
import java.net.InetAddress
import java.net.URI

@Serializable
internal data class ModuleConnection(
    val address: String,
    val secret: String,
    val certificate_sha256: String,
) {
    fun validate(): ModuleConnection {
        validateModuleAddress(address)
        require(secret.matches(secretPattern) && certificate_sha256.matches(fingerprintPattern))
        return this
    }

}

private val secretPattern = Regex("[0-9a-fA-F]{64}")
private val fingerprintPattern = Regex("[0-9a-f]{64}")
private val ipv4Pattern = Regex("[0-9]{1,3}(\\.[0-9]{1,3}){3}")

internal fun validateModuleAddress(address: String) {
    val uri = URI(address)
    require(uri.scheme == "https" && uri.userInfo == null && uri.rawQuery == null && uri.rawFragment == null)
    require(uri.path.isNullOrEmpty() && uri.port in 1..65535)
    val host = requireNotNull(uri.host).removeSurrounding("[", "]")
    require((host.contains(':') && host.all { it.isDigit() || it in ":.abcdefABCDEF" }
        || host.matches(ipv4Pattern)) && InetAddress.getByName(host).isLoopbackAddress)
}

internal class ConnectionStore private constructor(private val state: State) {
    constructor(context: Context) : this(sharedState(File(context.noBackupFilesDir, "module-connection.json")))

    internal constructor(readContent: () -> String?, persistContent: (String) -> Unit) :
        this(State(readContent, persistContent))

    fun read(): ModuleConnection? {
        if (state.loaded) return state.current
        return synchronized(state) {
            if (!state.loaded) {
                state.current = state.readContent()?.let(::decode)
                state.loaded = true
            }
            state.current
        }
    }

    fun invalidate(expected: ModuleConnection? = null): Boolean = synchronized(state) {
        // 失败请求只能清除自己使用的交付版本，不能清掉并发 Provider 写入的新凭据。
        if (expected != null && state.current !== expected) return@synchronized false
        state.current = null
        state.loaded = true
        true
    }

    fun write(content: String) = synchronized(state) {
        val connection = decode(content)
        state.persistContent(content)
        state.current = connection
        state.loaded = true
    }

    private class State(val readContent: () -> String?, val persistContent: (String) -> Unit) {
        @Volatile var current: ModuleConnection? = null
        @Volatile var loaded = false
    }

    private companion object {
        val states = HashMap<String, State>()

        fun decode(content: String): ModuleConnection {
            require(content.length <= 4096)
            return Json.decodeFromString<ModuleConnection>(content).validate()
        }

        fun sharedState(path: File): State = synchronized(states) {
            states.getOrPut(path.absolutePath) {
                val file = AtomicFile(path)
                State(
                    readContent = {
                        val input = try { file.openRead() } catch (_: FileNotFoundException) { null }
                        input?.use { it.bufferedReader().readText() }
                    },
                    persistContent = { content ->
                        val output = file.startWrite()
                        try {
                            output.write(content.toByteArray(Charsets.UTF_8))
                            file.finishWrite(output)
                        } catch (error: Throwable) {
                            file.failWrite(output)
                            throw error
                        }
                    },
                )
            }
        }
    }
}
