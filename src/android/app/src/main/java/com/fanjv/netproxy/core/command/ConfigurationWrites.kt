package com.fanjv.netproxy.core.command

import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.CoroutineStart
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.launch

internal class ConfigurationWrites(
    private val scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.Main),
    private val reportFailure: (Exception) -> Unit = {}
) {
    private val pending = mutableMapOf<String, MutableSet<CompletableDeferred<Unit>>>()

    fun launch(
        target: String,
        write: suspend () -> Unit,
        afterWrite: suspend () -> Unit = {},
        onFailure: (Exception) -> Unit = {}
    ): Job {
        val document = target.substringBefore('/')
        val completed = CompletableDeferred<Unit>()
        synchronized(pending) { pending.getOrPut(document) { mutableSetOf() }.add(completed) }
        fun release() {
            synchronized(pending) {
                pending[document]?.let { if (it.remove(completed) && it.isEmpty()) pending.remove(document) }
            }
            completed.complete(Unit)
        }
        // 先登记再调度，立即重进也能等待尚未开始的写入。
        val job = scope.launch(start = CoroutineStart.LAZY) {
            var failure: Exception? = null
            try {
                try {
                    write()
                } catch (error: Exception) {
                    if (error is CancellationException) throw error
                    failure = error
                    onFailure(error)
                } finally { release() }
                // 写入阶段结束后再确认，避免重新读取等待当前任务自身。
                afterWrite()
            } catch (error: Exception) {
                if (error is CancellationException) throw error
                if (failure == null) failure = error
                onFailure(error)
            }
            failure?.let(reportFailure)
        }
        job.invokeOnCompletion { release() }
        job.start()
        return job
    }

    suspend fun await(target: String) {
        while (true) {
            val writes = synchronized(pending) { pending[target.substringBefore('/')]?.toList().orEmpty() }
            if (writes.isEmpty()) return
            writes.forEach { it.await() }
        }
    }
}
