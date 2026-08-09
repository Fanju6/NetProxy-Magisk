package com.fanjv.netproxy.core.command

import com.fanjv.netproxy.core.module.ModulePaths
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.intOrNull
import kotlinx.serialization.json.jsonPrimitive

internal data class NetProxyCtlResponse(
    val schema: Int,
    val ok: Boolean,
    val code: String,
    val message: String,
    val data: JsonElement
)

internal class NetProxyCtlException(
    val resultCode: String,
    override val message: String,
    val data: JsonElement = JsonObject(emptyMap())
) : IllegalStateException(message)

internal data class NetProxyCtlOutput(
    val successful: Boolean,
    val stdout: List<String>,
    val stderr: List<String>
)

internal fun interface NetProxyCtlTransport {
    fun execute(arguments: List<String>, timeoutMillis: Long): NetProxyCtlOutput
}

private object RootShellNetProxyCtlTransport : NetProxyCtlTransport {
    override fun execute(arguments: List<String>, timeoutMillis: Long): NetProxyCtlOutput {
        val result = ShellCommand.exec(
            timeoutMillis,
            ModulePaths.NETPROXYCTL,
            "--json",
            "--timeout",
            "${(timeoutMillis + 999L) / 1000L}s",
            *arguments.toTypedArray()
        )
        return NetProxyCtlOutput(
            successful = result.successful,
            stdout = result.stdout,
            stderr = result.stderr
        )
    }
}

/** 严格解析 netproxyctl 的单一 JSON 输出，额外 stdout 会被视为契约错误。 */
internal class NetProxyCtlCodec(
    private val json: Json
) {
    fun decode(output: NetProxyCtlOutput): NetProxyCtlResponse {
        val payload = output.stdout.joinToString("\n").trim()
        if (payload.isEmpty()) {
            throw NetProxyCtlException(
                resultCode = "transport.invalid_output",
                message = output.stderr.lastOrNull()?.takeIf(String::isNotBlank)
                    ?: "模块没有返回有效的管理接口数据"
            )
        }

        val root = runCatching { json.parseToJsonElement(payload) as JsonObject }
            .getOrElse {
                throw NetProxyCtlException(
                    resultCode = "transport.invalid_json",
                    message = "模块返回的数据格式无效"
                )
            }
        val schema = root["schema"]?.jsonPrimitive?.intOrNull ?: 0
        val ok = root["ok"]?.jsonPrimitive?.booleanOrNull ?: false
        val code = root["code"]?.jsonPrimitive?.content.orEmpty()
        val message = root["message"]?.jsonPrimitive?.content.orEmpty()
        val data = root["data"] ?: JsonObject(emptyMap())

        if (schema != CONTRACT_SCHEMA) {
            throw NetProxyCtlException(
                resultCode = "transport.unsupported_schema",
                message = "模块管理接口版本不受支持"
            )
        }
        if (!ok || !output.successful) {
            throw NetProxyCtlException(
                resultCode = code.ifBlank { "command.failed" },
                message = message.ifBlank {
                    output.stderr.lastOrNull()?.takeIf(String::isNotBlank) ?: "模块操作失败"
                },
                data = data
            )
        }
        return NetProxyCtlResponse(schema, ok, code, message, data)
    }

    private companion object {
        const val CONTRACT_SCHEMA = 1
    }
}

/** 调用模块唯一管理入口，并严格解析 schema=1 JSON 契约。 */
internal class NetProxyCtlClient(
    internal val json: Json = Json {
        ignoreUnknownKeys = true
        explicitNulls = false
    },
    private val transport: NetProxyCtlTransport = RootShellNetProxyCtlTransport
) {
    private val codec = NetProxyCtlCodec(json)

    suspend fun execute(vararg args: String): NetProxyCtlResponse = withContext(Dispatchers.IO) {
        val arguments = args.toList()
        val timeoutMillis =
            if (arguments.firstOrNull() == "service" && arguments.getOrNull(1) == "start") {
                SERVICE_START_TIMEOUT_MILLIS
            } else {
                DEFAULT_TIMEOUT_MILLIS
            }
        codec.decode(transport.execute(arguments, timeoutMillis))
    }

    private companion object {
        const val DEFAULT_TIMEOUT_MILLIS = 30_000L
        const val SERVICE_START_TIMEOUT_MILLIS = 120_000L
    }
}
