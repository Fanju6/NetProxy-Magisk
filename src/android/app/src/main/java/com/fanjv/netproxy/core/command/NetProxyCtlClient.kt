package com.fanjv.netproxy.core.command

import com.fanjv.netproxy.core.module.ModulePaths
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.NonCancellable
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
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
) : IllegalStateException(message) {
    val persisted: Boolean
        get() = ((data as? JsonObject)?.get("persisted") as? JsonPrimitive)?.booleanOrNull == true
}

internal data class NetProxyCtlOutput(
    val successful: Boolean,
    val stdout: List<String>,
    val stderr: List<String>
)

internal fun interface NetProxyCtlTransport {
    suspend fun execute(arguments: List<String>, timeoutMillis: Long): NetProxyCtlOutput
}

private object RootShellNetProxyCtlTransport : NetProxyCtlTransport {
    override suspend fun execute(arguments: List<String>, timeoutMillis: Long): NetProxyCtlOutput {
        val command = mutableListOf(ModulePaths.NETPROXYCTL, "--json")
        if (timeoutMillis > 0) {
            command += listOf("--timeout", "${(timeoutMillis + 999L) / 1000L}s")
        }
        command += arguments
        val result = ShellCommand.execute(*command.toTypedArray(), isolated = !isShortRead(arguments))
        return NetProxyCtlOutput(
            successful = result.isSuccess,
            stdout = result.out,
            stderr = result.err
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

    /** 通过唯一管理入口确认模块 CLI 可用，不读取模块目录或进程状态。 */
    suspend fun isAvailable(): Boolean {
        return try {
            execute("service", "status")
            true
        } catch (error: CancellationException) {
            throw error
        } catch (error: NetProxyCtlException) {
            error.resultCode !in TRANSPORT_ERROR_CODES
        } catch (_: Exception) {
            false
        }
    }

    suspend fun execute(vararg args: String): NetProxyCtlResponse {
        val arguments = args.toList()
        val context = currentCoroutineContext()
        context.ensureActive()
        // 已提交的事务和输入文件必须等 Native 消费结束，取消等待不能终止收尾。
        val output = if (isShortRead(arguments)) {
            withContext(Dispatchers.IO) { transport.execute(arguments, commandTimeout(arguments)) }
        } else withContext(NonCancellable + Dispatchers.IO) {
            transport.execute(arguments, commandTimeout(arguments))
        }
        context.ensureActive()
        return withContext(Dispatchers.Default) { codec.decode(output) }
    }

    private companion object {
        val TRANSPORT_ERROR_CODES = setOf(
            "transport.invalid_output",
            "transport.invalid_json"
        )

    }
}

internal fun commandTimeout(arguments: List<String>): Long = when {
    arguments.firstOrNull() == "sub" && arguments.getOrNull(1) in setOf("add", "edit", "update", "update-all") -> 0L
    !isShortRead(arguments) -> 120_000L
    else -> 30_000L
}

internal fun isShortRead(arguments: List<String>): Boolean = when (arguments.firstOrNull()) {
    "service" -> arguments.getOrNull(1) == "status"
    "catalog", "node", "sub" -> arguments.getOrNull(1) in setOf("list", "get", "show", "snapshot", "export", "history", "progress", "status")
    "config" -> arguments.getOrNull(1) in setOf("list", "read")
    "app" -> arguments.getOrNull(1) == "list"
    "logs" -> arguments.getOrNull(1) == "show"
    else -> false
}
