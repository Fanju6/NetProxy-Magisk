/**
 * KernelSU API 封装层
 * 封装所有 KernelSU WebUI API，提供统一的 TypeScript 接口
 */
import {
    exec as ksuExec,
    spawn as ksuSpawn,
    fullScreen,
    enableEdgeToEdge,
    toast as ksuToast,
    moduleInfo,
    listPackages as ksuListPackages,
    getPackagesInfo as ksuGetPackagesInfo,
} from "kernelsu";

// ==================== 类型定义 ====================

/** Shell 执行选项 */
export interface ExecOptions {
    cwd?: string;
    env?: Record<string, string>;
    silent?: boolean;
}

/** Shell 执行结果 (原始) */
interface ExecResult {
    errno: number;
    stdout: string;
    stderr: string;
}

/** Spawn 进程事件流 */
export interface SpawnStream {
    on(event: "data", callback: (data: string) => void): void;
}

/** Spawn 子进程 */
export interface ChildProcess {
    stdout: SpawnStream;
    stderr?: SpawnStream;
    on(event: "exit", callback: (code: number) => void): void;
    on(event: "error", callback: (error: Error) => void): void;
}

/** 包信息 */
export interface PackageInfo {
    packageName: string;
    versionName: string;
    versionCode: number;
    appLabel: string;
    isSystem: boolean;
    uid: number;
}

/** 包列表类型 */
export type PackageType = "user" | "system" | "all";

// ==================== KernelSU API 封装 ====================

/**
 * KernelSU API 服务
 * 提供对 KernelSU WebUI 库的完整封装
 */
export class KSU {
    /** 模块路径常量 */
    static readonly MODULE_PATH = "/data/adb/modules/netproxy";

    // ==================== Shell 命令 ====================

    /**
     * 执行 Shell 命令
     * @param command 要执行的命令
     * @param options 执行选项
     * @returns stdout 内容 (已 trim)
     */
    static async exec(
        command: string,
        options: ExecOptions = {},
    ): Promise<string> {
        try {
            const { errno, stdout, stderr } = (await ksuExec(
                command,
                options,
            )) as ExecResult;
            if (errno !== 0) {
                throw new Error(stderr || `Command failed with code ${errno}`);
            }
            return stdout.trim();
        } catch (error) {
            console.error("[KSU.exec]", command, error);
            throw error;
        }
    }

    /**
     * 启动子进程 (非阻塞)
     * @param command 命令
     * @param args 参数列表
     * @param options 执行选项
     * @returns ChildProcess 实例
     */
    static spawn(
        command: string,
        args: string[] = [],
        options: ExecOptions = {},
    ): ChildProcess {
        // 自定义 spawn 实现，解决 WebViewInterface 在 exit 后发送 error 导致的 ReferenceError 问题
        const id = Date.now() + "_" + Math.floor(Math.random() * 1000);
        const callbackName = "spawn_callback_" + id;

        // 创建简单的事件发射器
        const createEmitter = () => {
            const listeners: Record<string, Function[]> = {};
            return {
                listeners,
                on(event: string, cb: Function) {
                    if (!listeners[event]) listeners[event] = [];
                    listeners[event].push(cb);
                },
                emit(event: string, ...data: any[]) {
                    if (listeners[event]) {
                        listeners[event].forEach((cb) => {
                            try {
                                cb(...data);
                            } catch (e) {
                                console.error(`Error in ${callbackName} ${event}`, e);
                            }
                        });
                    }
                },
            };
        };

        const stdout = createEmitter();
        const stderr = createEmitter();
        const mainEmitter = createEmitter();

        const child: any = {
            stdout: {
                on: (event: string, cb: any) => stdout.on(event, cb),
                // 供 Native 调用
                emit: stdout.emit,
            },
            stderr: {
                on: (event: string, cb: any) => stderr.on(event, cb),
                // 供 Native 调用
                emit: stderr.emit,
            },
            on: (event: string, cb: any) => mainEmitter.on(event, cb),
            // 供 Native 调用
            emit: mainEmitter.emit,
        };

        // 挂载到 window 供 Native 调用
        (window as any)[callbackName] = child;

        // 监听 exit 事件，延迟清理以防止 Native 在 exit 后继续发送 error
        child.on("exit", () => {
            setTimeout(() => {
                delete (window as any)[callbackName];
            }, 5000); // 保留 5 秒足够处理后续事件
        });

        // 尝试调用 Native 接口
        try {
            const ksu = (window as any).ksu;
            if (ksu && ksu.spawn) {
                ksu.spawn(
                    command,
                    JSON.stringify(args),
                    JSON.stringify(options),
                    callbackName,
                );
            } else {
                console.warn(
                    "[KSU] Native spawn not found, falling back to kernelsu package if possible or failing",
                );

                setTimeout(
                    () =>
                        child.emit(
                            "error",
                            new Error("Native ksu.spawn interface missing"),
                        ),
                    0,
                );
            }
        } catch (e: any) {
            console.error("[KSU] Spawn invocation failed", e);
            setTimeout(() => child.emit("error", e), 0);
        }

        return child as ChildProcess;
    }

    // ==================== WebView 控制 ====================

    /**
     * 设置全屏模式
     */
    static setFullScreen(enable: boolean): void {
        fullScreen(enable);
    }

    /**
     * 设置 WebView 内边距
     */
    static setInsets(enable: boolean): void {
        enableEdgeToEdge(enable);
    }

    // ==================== Toast ====================

    /**
     * 显示 Toast 消息
     */
    static showToast(message: string): void {
        ksuToast(message);
    }

    // ==================== 模块信息 ====================

    /**
     * 获取当前模块信息
     */
    static getModuleInfo(): string {
        return moduleInfo();
    }

    // ==================== 包管理 ====================

    /**
     * 列出已安装的包
     * @param type 包类型: 'user' | 'system' | 'all'
     */
    static listPackages(type: PackageType = "user"): string[] {
        return ksuListPackages(type) as string[];
    }

    /**
     * 获取包详情
     * @param packages 包名列表
     */
    static getPackagesInfo(packages: string[]): PackageInfo[] {
        return ksuGetPackagesInfo(packages) as PackageInfo[];
    }

    /**
     * 获取应用图标 URL
     * @param packageName 包名
     */
    static getAppIconUrl(packageName: string): string {
        return `ksu://icon/${packageName}`;
    }

    // ==================== 网络便捷方法 ====================

    /**
     * 使用 curl 获取 URL 内容
     */
    static async fetchUrl(url: string): Promise<string | null> {
        try {
            const result = await this.exec(
                `curl -sL --connect-timeout 10 --max-time 30 '${url}'`,
            );
            return result.trim() || null;
        } catch (error) {
            console.error("[KSU.fetchUrl]", url, error);
            return null;
        }
    }

    // ==================== 便捷方法 ====================

    /**
     * 使用 spawn 执行命令并等待结果 (非阻塞)
     * 适用于可能长时间运行的命令
     * @param command 命令
     * @param args 参数
     * @param timeoutMs 超时时间 (ms)
     */
    static spawnAsync(
        command: string,
        args: string[] = [],
        timeoutMs: number = 5000,
    ): Promise<{ code: number; stdout: string }> {
        return new Promise((resolve) => {
            let output = "";
            let resolved = false;

            const timeout = setTimeout(() => {
                if (!resolved) {
                    resolved = true;
                    resolve({ code: -1, stdout: output });
                }
            }, timeoutMs);

            try {
                const proc = this.spawn(command, args);

                proc.stdout.on("data", (data: string) => {
                    output += data;
                });

                proc.on("exit", (code: number) => {
                    if (resolved) return;
                    resolved = true;
                    clearTimeout(timeout);
                    resolve({ code, stdout: output });
                });

                proc.on("error", () => {
                    if (resolved) return;
                    resolved = true;
                    clearTimeout(timeout);
                    resolve({ code: -1, stdout: output });
                });
            } catch {
                if (!resolved) {
                    resolved = true;
                    clearTimeout(timeout);
                    resolve({ code: -1, stdout: "" });
                }
            }
        });
    }
}

// 导出别名以保持兼容性
export const ShellService = KSU;
