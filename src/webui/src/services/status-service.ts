import { KSU, ChildProcess } from "./ksu.js";

interface ServiceStatus {
  status: "running" | "stopped" | "unknown";
  config: string;
}

interface NetworkSpeed {
  download: string;
  upload: string;
}

interface TrafficStats {
  rx: number;
  tx: number;
}

interface InternalIP {
  ip: string;
  iface: string;
}

interface ExternalIPInfo {
  ip: string;
  countryCode: string;
}

interface XrayRule {
  type: string;
  inboundTag?: string[];
  outboundTag: string;
  port?: string;
  [key: string]: unknown;
}

/**
 * Status Service - 状态页面相关业务逻辑
 */
export class StatusService {
  // ==================== 服务控制 ====================

  // 获取服务状态
  static async getStatus(): Promise<ServiceStatus> {
    try {
      // 使用 pidof 检测 xray 进程是否运行
      const pidOutput = await KSU.exec(
        `pidof -s /data/adb/modules/netproxy/bin/xray 2>/dev/null || echo`,
      );
      const isRunning = pidOutput.trim() !== "";
      const status = isRunning ? "running" : "stopped";

      // config 从 module.conf 读取
      const configOutput = await KSU.exec(
        `cat ${KSU.MODULE_PATH}/config/module.conf 2>/dev/null || echo`,
      );
      const config = configOutput.match(/CURRENT_CONFIG="([^"]*)"/)?.[1] || "";

      return { status, config: config.split("/").pop() || "" };
    } catch (error) {
      return { status: "unknown", config: "" };
    }
  }

  // 启动服务（非阻塞）
  static async startService(): Promise<boolean> {
    // 后台执行服务脚本，不等待完成 (fire-and-forget)
    KSU.spawn("su", [
      "-c",
      `sh ${KSU.MODULE_PATH}/scripts/core/service.sh start >/dev/null 2>&1 &`,
    ]);
    // 轮询等待服务启动
    return await this.pollServiceStatus("running", 15000);
  }

  // 停止服务（非阻塞）
  static async stopService(): Promise<boolean> {
    // 后台执行服务脚本，不等待完成 (fire-and-forget)
    KSU.spawn("su", [
      "-c",
      `sh ${KSU.MODULE_PATH}/scripts/core/service.sh stop >/dev/null 2>&1 &`,
    ]);
    // 轮询等待服务停止
    return await this.pollServiceStatus("stopped", 10000);
  }

  // 轮询服务状态
  static async pollServiceStatus(
    targetStatus: string,
    timeout: number,
  ): Promise<boolean> {
    const start = Date.now();
    const interval = 500; // 每 500ms 检查一次

    while (Date.now() - start < timeout) {
      await new Promise((resolve) => setTimeout(resolve, interval));
      try {
        const { status } = await this.getStatus();
        if (status === targetStatus) {
          return true;
        }
      } catch (e) {
        // 忽略检查过程中的错误
      }
    }
    return false; // 超时
  }

  // ==================== 状态监控 ====================

  // 获取服务运行时间
  static async getUptime(): Promise<string> {
    try {
      const result = await KSU.exec(`
                 pid=$(pidof xray) || exit 1
                 awk 'BEGIN {
                     getline u < "/proc/uptime"; split(u, a, " ")
                     getline s < "/proc/'"$pid"'/stat"; split(s, b, " ")
                     "getconf CLK_TCK" | getline h
                     t = int(a[1] - b[22] / h)
                     d = int(t / 86400); h = int((t % 86400) / 3600); m = int((t % 3600) / 60); s = t % 60
                     if (d > 0) printf "%d-%02d:%02d:%02d", d, h, m, s
                     else printf "%02d:%02d:%02d", h, m, s
                 }'
             `);
      return result || "--";
    } catch (error) {
      return "--";
    }
  }

  // 缓存上次网络数据
  static _lastNetBytes: { rx: number; tx: number } | null = null;
  static _lastNetTime = 0;

  // 获取实时网速（无阻塞）
  static async getNetworkSpeed(): Promise<NetworkSpeed> {
    try {
      const result = await KSU.exec(
        `awk '/:/ {rx+=$2; tx+=$10} END {print rx, tx}' /proc/net/dev`,
      );
      const [rx, tx] = result.split(/\s+/).map(Number);
      const now = Date.now();

      if (this._lastNetBytes === null) {
        // 首次调用，保存数据，返回 0
        this._lastNetBytes = { rx, tx };
        this._lastNetTime = now;
        return { download: "0 KB/s", upload: "0 KB/s" };
      }

      const elapsed = (now - this._lastNetTime) / 1000; // 秒
      if (elapsed < 0.5) {
        // 间隔太短，返回上次值
        return { download: "0 KB/s", upload: "0 KB/s" };
      }

      const download = Math.max(
        0,
        Math.floor((rx - this._lastNetBytes.rx) / 1024 / elapsed),
      );
      const upload = Math.max(
        0,
        Math.floor((tx - this._lastNetBytes.tx) / 1024 / elapsed),
      );

      this._lastNetBytes = { rx, tx };
      this._lastNetTime = now;

      return { download: `${download} KB/s`, upload: `${upload} KB/s` };
    } catch (error) {
      return { download: "0 KB/s", upload: "0 KB/s" };
    }
  }

  // 获取流量统计 (今日累计)
  static async getTrafficStats(): Promise<TrafficStats> {
    try {
      // 获取所有接口的总流量
      const result = await KSU.exec(
        `awk '/:/ {rx+=$2; tx+=$10} END {print rx, tx}' /proc/net/dev`,
      );
      const parts = result.split(/\s+/);
      return {
        rx: parseInt(parts[0]) || 0,
        tx: parseInt(parts[1]) || 0,
      };
    } catch (error) {
      return { rx: 0, tx: 0 };
    }
  }

  // ==================== 系统状态监控 ====================

  static _lastSystemCpuTime: number | null = null;
  static _lastProcessCpuTime: number | null = null;
  static _lastProcessId: string | null = null;

  // 获取系统状态 (CPU/内存)
  // 获取 Xray 进程状态 (CPU/内存)
  static async getSystemStatus(): Promise<{
    cpu: number;
    mem: { total: number; used: number; percentage: number };
  }> {
    try {
      // 1. 获取 PID
      const pidArg = `/data/adb/modules/netproxy/bin/xray`;
      const pidResult = await KSU.exec(
        `pidof -s ${pidArg} 2>/dev/null || echo`,
      );
      const pid = pidResult.trim();

      if (!pid) {
        return { cpu: 0, mem: { total: 0, used: 0, percentage: 0 } };
      }

      // 2. 获取内存 (VmRSS)
      const statusResult = await KSU.exec(
        `grep VmRSS /proc/${pid}/status 2>/dev/null`,
      );
      let memUsed = 0; // Bytes
      if (statusResult) {
        const match = statusResult.match(/VmRSS:\s+(\d+)\s+kB/);
        if (match) {
          memUsed = parseInt(match[1]) * 1024;
        }
      }

      // 获取总内存 (用于计算百分比)
      const memInfoResult = await KSU.exec(
        `grep MemTotal /proc/meminfo 2>/dev/null`,
      );
      let memTotal = 0;
      if (memInfoResult) {
        const match = memInfoResult.match(/MemTotal:\s+(\d+)\s+kB/);
        if (match) {
          memTotal = parseInt(match[1]) * 1024;
        }
      }

      // 计算内存百分比 (Xray占用 / 总内存)
      const memPercentage =
        memTotal > 0 ? Math.round((memUsed / memTotal) * 1000) / 10 : 0;

      // 3. 获取 CPU 使用率
      /*
                ProcessCPU = utime + stime (from /proc/[pid]/stat)
                TotalSystemCPU = sum(fields) (from /proc/stat)
            */
      let cpuUsage = 0;
      const procStatRaw = await KSU.exec(`cat /proc/${pid}/stat 2>/dev/null`);
      const sysStatRaw = await KSU.exec(`cat /proc/stat | head -n 1`);

      if (procStatRaw && sysStatRaw) {
        const procParts = procStatRaw.trim().split(/\s+/);
        const sysParts = sysStatRaw.trim().split(/\s+/);

        if (procParts.length > 15 && sysParts.length > 8) {
          const utime = parseInt(procParts[13]);
          const stime = parseInt(procParts[14]);
          const processTime = utime + stime;

          const sysTotalTime = sysParts
            .slice(1)
            .reduce((acc, val) => acc + (parseInt(val) || 0), 0);

          if (
            this._lastProcessId === pid &&
            this._lastSystemCpuTime !== null &&
            this._lastProcessCpuTime !== null
          ) {
            const procDelta = processTime - this._lastProcessCpuTime;
            const sysDelta = sysTotalTime - this._lastSystemCpuTime;

            if (sysDelta > 0) {
              cpuUsage = (procDelta / sysDelta) * 100;
              // 保留一位小数
              cpuUsage = Math.min(
                100,
                Math.max(0, Math.round(cpuUsage * 10) / 10),
              );
            }
          }

          this._lastProcessId = pid;
          this._lastProcessCpuTime = processTime;
          this._lastSystemCpuTime = sysTotalTime;
        }
      }

      return {
        cpu: cpuUsage,
        mem: {
          total: memTotal,
          used: memUsed,
          percentage: memPercentage,
        },
      };
    } catch (error) {
      console.error("Failed to get Xray status:", error);
      return { cpu: 0, mem: { total: 0, used: 0, percentage: 0 } };
    }
  }

  // ==================== IP 信息 ====================

  // 获取内网IP
  static async getInternalIP(): Promise<InternalIP[]> {
    try {
      const result = await KSU.exec(
        `ip -4 addr show 2>/dev/null | awk '/inet / && !/127\\.0\\.0\\.1/ {gsub(/\\/.*/, "", $2); print $2, $NF}' | head -3`,
      );
      // 解析格式: "192.168.1.100 wlan0"
      return result
        .split("\n")
        .filter((l) => l.trim())
        .map((line) => {
          const parts = line.trim().split(/\s+/);
          return { ip: parts[0], iface: parts[1] || "unknown" };
        })
        .filter((item) => item.ip);
    } catch (error) {
      return [];
    }
  }

  // 获取外网IP信息 (IP + 国家代码)
  static async getExternalIPInfo(): Promise<ExternalIPInfo | null> {
    // 定义支持国家代码的 API 配置
    const ipApis = [
      { url: "https://ipwho.is", ipField: "ip", countryField: "country_code" },
      {
        url: "https://api.ip.sb/geoip",
        ipField: "ip",
        countryField: "country_code",
      },
      {
        url: "https://ipapi.co/json",
        ipField: "ip",
        countryField: "country_code",
      },
      {
        url: "http://ip-api.com/json",
        ipField: "query",
        countryField: "countryCode",
      },
    ];

    const fetchPromises = ipApis.map((api) => {
      return new Promise<ExternalIPInfo>((resolve, reject) => {
        let output = "";
        let resolved = false;

        const timeout = setTimeout(() => {
          if (!resolved) {
            resolved = true;
            reject(new Error("timeout"));
          }
        }, 5000);

        try {
          const curl = KSU.spawn("curl", [
            "-s",
            "--connect-timeout",
            "3",
            "--max-time",
            "5",
            api.url,
          ]);

          curl.stdout.on("data", (data: string) => {
            output += data;
          });

          curl.on("exit", (code: number) => {
            if (resolved) return;
            resolved = true;
            clearTimeout(timeout);

            if (code === 0 && output.trim()) {
              try {
                const json = JSON.parse(output.trim());
                const ip = json[api.ipField];
                const countryCode = json[api.countryField];

                // 验证 IP 格式
                if (
                  ip &&
                  typeof ip === "string" &&
                  /^[\d.:a-fA-F]+$/.test(ip)
                ) {
                  // 验证国家代码（2位大写字母）
                  if (
                    countryCode &&
                    typeof countryCode === "string" &&
                    /^[A-Z]{2}$/.test(countryCode)
                  ) {
                    resolve({ ip, countryCode });
                    return;
                  }
                }
              } catch {
                // JSON parse failed
              }
            }
            reject(new Error("failed"));
          });

          curl.on("error", () => {
            if (resolved) return;
            resolved = true;
            clearTimeout(timeout);
            reject(new Error("error"));
          });
        } catch {
          if (!resolved) {
            resolved = true;
            clearTimeout(timeout);
            reject(new Error("spawn failed"));
          }
        }
      });
    });

    return Promise.any(fetchPromises).catch(() => null);
  }

  // Ping 延迟测试 (使用 spawn 非阻塞)
  static getPingLatency(host: string): Promise<string> {
    return new Promise((resolve) => {
      let output = "";
      let resolved = false;

      const timeout = setTimeout(() => {
        if (!resolved) {
          resolved = true;
          resolve("timeout");
        }
      }, 3000);

      try {
        const ping = KSU.spawn("ping", ["-c", "1", "-W", "2", host]);

        ping.stdout.on("data", (data: string) => {
          output += data;
        });

        ping.on("exit", (code: number) => {
          if (resolved) return;
          resolved = true;
          clearTimeout(timeout);

          if (code === 0 && output) {
            const match = output.match(/time=([\d.]+)\s*ms/);
            if (match) {
              resolve(`${Math.round(parseFloat(match[1]))} ms`);
              return;
            }
          }
          resolve("timeout");
        });

        ping.on("error", () => {
          if (resolved) return;
          resolved = true;
          clearTimeout(timeout);
          resolve("failed");
        });
      } catch {
        if (!resolved) {
          resolved = true;
          clearTimeout(timeout);
          resolve("failed");
        }
      }
    });
  }

  // ==================== 出站模式 ====================

  // 获取当前出站模式
  static async getOutboundMode(): Promise<string> {
    try {
      const output = await KSU.exec(
        `grep '^OUTBOUND_MODE=' ${KSU.MODULE_PATH}/config/module.conf 2>/dev/null | cut -d'=' -f2`,
      );
      return output.trim() || "rule";
    } catch (error) {
      return "rule";
    }
  }

  // 设置出站模式
  static async setOutboundMode(mode: string): Promise<boolean> {
    try {
      let rulesFile = "";

      if (mode === "global") {
        const rulesJson = await this.generateGlobalRules();
        rulesFile = `${KSU.MODULE_PATH}/logs/.mode_rules.json`;
        const base64 = btoa(
          unescape(encodeURIComponent(JSON.stringify(rulesJson, null, 2))),
        );
        await KSU.exec(`echo '${base64}' | base64 -d > ${rulesFile}`);
      } else if (mode === "direct") {
        const rulesJson = await this.generateDirectRules();
        rulesFile = `${KSU.MODULE_PATH}/logs/.mode_rules.json`;
        const base64 = btoa(
          unescape(encodeURIComponent(JSON.stringify(rulesJson, null, 2))),
        );
        await KSU.exec(`echo '${base64}' | base64 -d > ${rulesFile}`);
      }

      const result = await KSU.exec(
        `sh ${KSU.MODULE_PATH}/scripts/core/switch-mode.sh ${mode} ${rulesFile}`,
      );

      if (rulesFile) {
        await KSU.exec(`rm -f ${rulesFile}`).catch(() => {});
      }

      return result.includes("success");
    } catch (error) {
      console.error("设置出站模式失败:", error);
      return false;
    }
  }

  static async generateGlobalRules(): Promise<{
    routing: { domainStrategy: string; rules: XrayRule[] };
  }> {
    return {
      routing: {
        domainStrategy: "AsIs",
        rules: [
          {
            type: "field",
            inboundTag: ["tproxy-in"],
            port: "53",
            outboundTag: "dns-out",
          },
          { type: "field", port: "0-65535", outboundTag: "proxy" },
          {
            type: "field",
            inboundTag: ["domestic-dns"],
            outboundTag: "direct",
          },
          { type: "field", inboundTag: ["dns-module"], outboundTag: "proxy" },
        ],
      },
    };
  }

  static async generateDirectRules(): Promise<{
    routing: { domainStrategy: string; rules: XrayRule[] };
  }> {
    return {
      routing: {
        domainStrategy: "AsIs",
        rules: [
          {
            type: "field",
            inboundTag: ["tproxy-in"],
            port: "53",
            outboundTag: "dns-out",
          },
          { type: "field", port: "0-65535", outboundTag: "direct" },
          {
            type: "field",
            inboundTag: ["domestic-dns"],
            outboundTag: "direct",
          },
          { type: "field", inboundTag: ["dns-module"], outboundTag: "direct" },
        ],
      },
    };
  }
}
