import { KSU } from "./ksu.js";

/** 模块设置接口 */
export interface ModuleSettings {
  auto_start: boolean;
  oneplus_a16_fix: boolean;
}

/** 代理设置接口 */
export interface ProxySettings {
  // Core
  proxy_mode: number;
  proxy_tcp_port: string;
  proxy_udp_port: string;
  routing_mark: string;
  mark_value: number;
  mark_value6: number;
  table_id: number;

  // Interfaces
  mobile_interface: string;
  wifi_interface: string;
  hotspot_interface: string;
  usb_interface: string;
  other_proxy_interfaces: string;
  other_bypass_interfaces: string;

  // Switches
  proxy_mobile: boolean;
  proxy_wifi: boolean;
  proxy_hotspot: boolean;
  proxy_usb: boolean;
  proxy_tcp: boolean;
  proxy_udp: boolean;
  proxy_ipv6: boolean;
  force_mark_bypass: boolean;
  block_quic: boolean;
  compatibility_mode: boolean;

  // DNS
  dns_hijack_enable: boolean;
  dns_port: string;

  // IP Lists & Rules
  bypass_cn_ip: boolean;
  bypass_ipv4_list: string;
  bypass_ipv6_list: string;
  // 中国 IP 配置
  cn_ip_file: string;
  cn_ipv6_file: string;
  cn_ip_url: string;
  cn_ipv6_url: string;
  proxy_ipv4_list: string;
  proxy_ipv6_list: string;

  // Mac 过滤
  mac_filter_enable: boolean;
  mac_proxy_mode: string;
  proxy_macs_list: string;
  bypass_macs_list: string;

  [key: string]: any;
}

/** DNS 配置接口 */
export interface DnsConfig {
  dns: {
    hosts: Record<string, string | string[]>;
    servers: (string | DnsServerConfig)[];
    [key: string]: unknown;
  };
}

export interface DnsServerConfig {
  address: string;
  port?: number;
  domains?: string[];
  [key: string]: unknown;
}

/** 路由规则接口 */
export interface RoutingRule {
  name?: string;
  type?: string;
  domain?: string;
  ip?: string;
  port?: string;
  protocol?: string;
  network?: string;
  inboundTag?: string;
  outboundTag?: string;
  enabled?: boolean;
}

/** Xray 路由规则 */
interface XrayRule {
  type: string;
  domain?: string[];
  ip?: string[];
  port?: string;
  protocol?: string[];
  network?: string;
  inboundTag?: string[];
  outboundTag: string;
}

/** 操作结果接口 */
interface OperationResult {
  success: boolean;
  error?: string;
  path?: string;
  output?: string;
  isLatest?: boolean;
  message?: string;
}

/**
 * Settings Service - 设置页面相关业务逻辑
 */
export class SettingsService {
  // ===================== 模块设置 =====================

  /** 获取模块设置 */
  static async getModuleSettings(): Promise<ModuleSettings> {
    try {
      const content = await KSU.exec(
        `cat ${KSU.MODULE_PATH}/config/module.conf`,
      );
      const settings: Partial<ModuleSettings> = {};

      const autoStartMatch = content.match(/AUTO_START=(\d+)/);
      settings.auto_start = autoStartMatch ? autoStartMatch[1] === "1" : true;

      const oneplusFixMatch = content.match(/ONEPLUS_A16_FIX=(\d+)/);
      settings.oneplus_a16_fix = oneplusFixMatch
        ? oneplusFixMatch[1] === "1"
        : true;

      return settings as ModuleSettings;
    } catch (error) {
      return {
        auto_start: true,
        oneplus_a16_fix: true,
      };
    }
  }

  /** 设置模块选项 */
  static async setModuleSetting(
    key: string,
    value: boolean,
  ): Promise<OperationResult> {
    const upperKey = key.toUpperCase();
    const numValue = value ? "1" : "0";
    await KSU.exec(
      `sed -i 's/${upperKey}=.*/${upperKey}=${numValue}/' ${KSU.MODULE_PATH}/config/module.conf`,
    );
    return { success: true };
  }

  // 执行 OnePlus A16 兼容性修复脚本
  static async executeOneplusFix() {
    await KSU.exec(
      `su -c "sh ${KSU.MODULE_PATH}/scripts/utils/oneplus_a16_fix.sh"`,
    );
    return { success: true };
  }

  // 打开外部浏览器
  static async openExternalUrl(url) {
    await KSU.exec(`am start -a android.intent.action.VIEW -d "${url}"`);
  }

  // ===================== 代理开关设置 =====================

  // 获取完整代理设置
  static async getProxySettings(): Promise<ProxySettings> {
    try {
      const content = await KSU.exec(
        `cat ${KSU.MODULE_PATH}/config/tproxy.conf`,
      );
      const settings: any = {};

      const parseLine = (key: string, type: "bool" | "number" | "string") => {
        // 匹配 KEY="value" 或 KEY=value
        const regex = new RegExp(`^${key}=["']?(.*?)["']?$`, "m");
        const match = content.match(regex);
        if (match) {
          const val = match[1];
          if (type === "bool") return val === "1";
          if (type === "number") return Number(val);
          return val;
        }
        return type === "bool" ? false : type === "number" ? 0 : "";
      };

      // 映射键值
      settings.proxy_mode = parseLine("PROXY_MODE", "number");
      settings.proxy_tcp_port = parseLine("PROXY_TCP_PORT", "string");
      settings.proxy_udp_port = parseLine("PROXY_UDP_PORT", "string");
      settings.routing_mark = parseLine("ROUTING_MARK", "string");
      settings.mark_value = parseLine("MARK_VALUE", "number");
      settings.mark_value6 = parseLine("MARK_VALUE6", "number");
      settings.table_id = parseLine("TABLE_ID", "number");

      settings.mobile_interface = parseLine("MOBILE_INTERFACE", "string");
      settings.wifi_interface = parseLine("WIFI_INTERFACE", "string");
      settings.hotspot_interface = parseLine("HOTSPOT_INTERFACE", "string");
      settings.usb_interface = parseLine("USB_INTERFACE", "string");
      settings.other_proxy_interfaces = parseLine(
        "OTHER_PROXY_INTERFACES",
        "string",
      );
      settings.other_bypass_interfaces = parseLine(
        "OTHER_BYPASS_INTERFACES",
        "string",
      );

      settings.proxy_mobile = parseLine("PROXY_MOBILE", "bool");
      settings.proxy_wifi = parseLine("PROXY_WIFI", "bool");
      settings.proxy_hotspot = parseLine("PROXY_HOTSPOT", "bool");
      settings.proxy_usb = parseLine("PROXY_USB", "bool");
      settings.proxy_tcp = parseLine("PROXY_TCP", "bool");
      settings.proxy_udp = parseLine("PROXY_UDP", "bool");
      settings.proxy_ipv6 = parseLine("PROXY_IPV6", "bool");
      settings.force_mark_bypass = parseLine("FORCE_MARK_BYPASS", "bool");
      settings.block_quic = parseLine("BLOCK_QUIC", "bool");
      // 兼容模式 = 禁用性能模式
      settings.compatibility_mode = !parseLine("PERFORMANCE_MODE", "bool");

      settings.dns_hijack_enable = parseLine("DNS_HIJACK_ENABLE", "bool");
      settings.dns_port = parseLine("DNS_PORT", "string");

      // IP 列表与规则
      settings.bypass_cn_ip = parseLine("BYPASS_CN_IP", "bool");
      settings.cn_ip_file = parseLine("CN_IP_FILE", "string");
      settings.cn_ipv6_file = parseLine("CN_IPV6_FILE", "string");
      settings.cn_ip_url = parseLine("CN_IP_URL", "string");
      settings.cn_ipv6_url = parseLine("CN_IPV6_URL", "string");
      settings.bypass_ipv4_list = parseLine("BYPASS_IPv4_LIST", "string");
      settings.bypass_ipv6_list = parseLine("BYPASS_IPv6_LIST", "string");
      settings.proxy_ipv4_list = parseLine("PROXY_IPv4_LIST", "string");
      settings.proxy_ipv6_list = parseLine("PROXY_IPv6_LIST", "string");

      settings.mac_filter_enable = parseLine("MAC_FILTER_ENABLE", "bool");
      settings.mac_proxy_mode = parseLine("MAC_PROXY_MODE", "string");
      settings.proxy_macs_list = parseLine("PROXY_MACS_LIST", "string");
      settings.bypass_macs_list = parseLine("BYPASS_MACS_LIST", "string");

      return settings as ProxySettings;
    } catch (error) {
      console.error("Failed to load proxy settings", error);
      return {} as ProxySettings;
    }
  }

  // 保存代理设置
  static async saveProxySettings(
    settings: ProxySettings,
  ): Promise<OperationResult> {
    try {
      // 读取当前文件内容以保留注释
      let content = await KSU.exec(`cat ${KSU.MODULE_PATH}/config/tproxy.conf`);

      const updateKey = (key: string, value: any, isString = false) => {
        let valStr = String(value);
        if (typeof value === "boolean") valStr = value ? "1" : "0";

        // 如果是字符串类型或包含空格，加上双引号
        if (isString || valStr.includes(" ")) {
          valStr = `"${valStr}"`;
        }

        const regex = new RegExp(`^${key}=.*$`, "m");
        if (regex.test(content)) {
          content = content.replace(regex, `${key}=${valStr}`);
        } else {
          // 如果 key 不存在，追加到文件末尾 (通常不会发生，除非是在很老的配置文件上)
          content += `\n${key}=${valStr}`;
        }
      };

      updateKey("PROXY_MODE", settings.proxy_mode);
      updateKey("PROXY_TCP_PORT", settings.proxy_tcp_port, true);
      updateKey("PROXY_UDP_PORT", settings.proxy_udp_port, true);
      updateKey("ROUTING_MARK", settings.routing_mark, true);
      updateKey("MARK_VALUE", settings.mark_value);
      updateKey("MARK_VALUE6", settings.mark_value6);
      updateKey("TABLE_ID", settings.table_id);

      updateKey("MOBILE_INTERFACE", settings.mobile_interface, true);
      updateKey("WIFI_INTERFACE", settings.wifi_interface, true);
      updateKey("HOTSPOT_INTERFACE", settings.hotspot_interface, true);
      updateKey("USB_INTERFACE", settings.usb_interface, true);
      updateKey(
        "OTHER_PROXY_INTERFACES",
        settings.other_proxy_interfaces,
        true,
      );
      updateKey(
        "OTHER_BYPASS_INTERFACES",
        settings.other_bypass_interfaces,
        true,
      );

      updateKey("PROXY_MOBILE", settings.proxy_mobile);
      updateKey("PROXY_WIFI", settings.proxy_wifi);
      updateKey("PROXY_HOTSPOT", settings.proxy_hotspot);
      updateKey("PROXY_USB", settings.proxy_usb);
      updateKey("PROXY_TCP", settings.proxy_tcp);
      updateKey("PROXY_UDP", settings.proxy_udp);
      updateKey("PROXY_IPV6", settings.proxy_ipv6);
      updateKey("FORCE_MARK_BYPASS", settings.force_mark_bypass);
      updateKey("BLOCK_QUIC", settings.block_quic);
      // 兼容模式 = 禁用性能模式
      updateKey("PERFORMANCE_MODE", !settings.compatibility_mode);

      updateKey("DNS_HIJACK_ENABLE", settings.dns_hijack_enable);
      updateKey("DNS_PORT", settings.dns_port, true);

      updateKey("BYPASS_CN_IP", settings.bypass_cn_ip);
      updateKey("CN_IP_FILE", settings.cn_ip_file, true);
      updateKey("CN_IPV6_FILE", settings.cn_ipv6_file, true);
      updateKey("CN_IP_URL", settings.cn_ip_url, true);
      updateKey("CN_IPV6_URL", settings.cn_ipv6_url, true);
      updateKey("BYPASS_IPv4_LIST", settings.bypass_ipv4_list, true);
      updateKey("BYPASS_IPv6_LIST", settings.bypass_ipv6_list, true);
      updateKey("PROXY_IPv4_LIST", settings.proxy_ipv4_list, true);
      updateKey("PROXY_IPv6_LIST", settings.proxy_ipv6_list, true);

      updateKey("MAC_FILTER_ENABLE", settings.mac_filter_enable);
      updateKey("MAC_PROXY_MODE", settings.mac_proxy_mode, true);
      updateKey("PROXY_MACS_LIST", settings.proxy_macs_list, true);
      updateKey("BYPASS_MACS_LIST", settings.bypass_macs_list, true);

      // 写入文件
      // 使用 base64 编码以处理特殊字符
      const base64 = btoa(unescape(encodeURIComponent(content)));
      await KSU.exec(
        `echo '${base64}' | base64 -d > ${KSU.MODULE_PATH}/config/tproxy.conf`,
      );

      return { success: true };
    } catch (error: any) {
      console.error("Failed to save proxy settings", error);
      return { success: false, error: error.message };
    }
  }

  // 设置代理开关
  static async setProxySetting(key: string, value: boolean) {
    try {
      const upperKey = key.toUpperCase();
      const numValue = value ? "1" : "0";
      await KSU.exec(
        `sed -i 's/${upperKey}=.*/${upperKey}=${numValue}/' ${KSU.MODULE_PATH}/config/tproxy.conf`,
      );
      return { success: true };
    } catch (error: any) {
      return { success: false, error: error.message };
    }
  }

  // ===================== DNS 配置管理 =====================

  // 获取 DNS 配置
  static async getDnsConfig() {
    try {
      const output = await KSU.exec(
        `cat ${KSU.MODULE_PATH}/config/xray/confdir/02_dns.json`,
      );
      return JSON.parse(output);
    } catch (error) {
      console.error("获取 DNS 配置失败:", error);
      return { dns: { hosts: {}, servers: [] } };
    }
  }

  // 保存 DNS 配置
  static async saveDnsConfig(config) {
    try {
      const json = JSON.stringify(config, null, 4);
      const base64 = btoa(unescape(encodeURIComponent(json)));
      await KSU.exec(
        `echo '${base64}' | base64 -d > ${KSU.MODULE_PATH}/config/xray/confdir/02_dns.json`,
      );
      return { success: true };
    } catch (error: any) {
      console.error("保存 DNS 配置失败:", error);
      return { success: false, error: error.message };
    }
  }

  // ===================== 路由规则管理 =====================

  // 获取路由规则列表
  static async getRoutingRules() {
    try {
      const output = await KSU.exec(
        `cat ${KSU.MODULE_PATH}/config/routing_rules.json`,
      );
      return JSON.parse(output);
    } catch (error) {
      console.error("获取路由规则失败:", error);
      return [];
    }
  }

  // 保存路由规则列表
  static async saveRoutingRules(rules) {
    try {
      const json = JSON.stringify(rules, null, 4);
      // 使用 base64 编码避免特殊字符问题
      const base64 = btoa(unescape(encodeURIComponent(json)));
      await KSU.exec(
        `echo '${base64}' | base64 -d > ${KSU.MODULE_PATH}/config/routing_rules.json`,
      );
      return { success: true };
    } catch (error: any) {
      console.error("保存路由规则失败:", error);
      return { success: false, error: error.message };
    }
  }

  // 应用路由规则（在前端生成 routing.json）
  static async applyRoutingRules(
    rules: RoutingRule[],
  ): Promise<OperationResult> {
    try {
      // 构建路由规则数组
      const xrayRules: XrayRule[] = [];

      for (const rule of rules) {
        if (rule.enabled === false) continue;

        const xrayRule: XrayRule = {
          type: "field",
          outboundTag: rule.outboundTag || "proxy",
        };

        // 处理 domain
        if (rule.domain) {
          xrayRule.domain = rule.domain.split(",").map((d) => {
            d = d.trim();
            if (
              d.startsWith("geosite:") ||
              d.startsWith("domain:") ||
              d.startsWith("full:") ||
              d.startsWith("regexp:")
            ) {
              return d;
            }
            return `domain:${d}`;
          });
        }

        // 处理 ip
        if (rule.ip) {
          xrayRule.ip = rule.ip.split(",").map((i) => i.trim());
        }

        // 处理 port
        if (rule.port) {
          xrayRule.port = rule.port.trim();
        }

        // 处理 protocol
        if (rule.protocol) {
          xrayRule.protocol = rule.protocol.split(",").map((p) => p.trim());
        }

        // 处理 network
        if (rule.network) {
          xrayRule.network = rule.network.trim();
        }

        xrayRules.push(xrayRule);
      }

      // 添加固定的内部 DNS 规则
      xrayRules.push({
        type: "field",
        inboundTag: ["domestic-dns"],
        outboundTag: "direct",
      });
      xrayRules.push({
        type: "field",
        inboundTag: ["dns-module"],
        outboundTag: "proxy",
      });

      // 构建完整的路由配置
      const routingConfig = {
        routing: {
          domainStrategy: "AsIs",
          rules: xrayRules,
        },
      };

      // 使用 base64 编码写入文件
      const json = JSON.stringify(routingConfig, null, 4);
      const base64 = btoa(unescape(encodeURIComponent(json)));
      await KSU.exec(
        `echo '${base64}' | base64 -d > ${KSU.MODULE_PATH}/config/xray/confdir/03_routing.json`,
      );

      return { success: true };
    } catch (error: any) {
      console.error("应用路由规则失败:", error);
      return { success: false, error: error.message };
    }
  }

  // ===================== 导出功能 =====================

  // 导出日志到 Download 目录
  static async exportLogs() {
    try {
      const timestamp = new Date()
        .toISOString()
        .replace(/[:-]/g, "")
        .replace("T", "_")
        .slice(0, 15);
      const filename = `netproxy_logs_${timestamp}.tar.gz`;
      const downloadPath = "/storage/emulated/0/Download";
      const outputPath = `${downloadPath}/${filename}`;

      // 确保 Download 目录存在
      await KSU.exec(`mkdir -p ${downloadPath}`);

      // 使用 tar 压缩 logs 文件夹
      await KSU.exec(`cd ${KSU.MODULE_PATH} && tar -czf ${outputPath} logs/`);

      return { success: true, path: outputPath };
    } catch (error: any) {
      console.error("导出日志失败:", error);
      return { success: false, error: error.message };
    }
  }

  // 导出日志与配置到 Download 目录
  static async exportAll() {
    try {
      const timestamp = new Date()
        .toISOString()
        .replace(/[:-]/g, "")
        .replace("T", "_")
        .slice(0, 15);
      const filename = `netproxy_backup_${timestamp}.tar.gz`;
      const downloadPath = "/storage/emulated/0/Download";
      const outputPath = `${downloadPath}/${filename}`;

      // 确保 Download 目录存在
      await KSU.exec(`mkdir -p ${downloadPath}`);

      // 使用 tar 同时压缩 logs 和 config 文件夹
      await KSU.exec(
        `cd ${KSU.MODULE_PATH} && tar -czf ${outputPath} logs/ config/`,
      );

      return { success: true, path: outputPath };
    } catch (error: any) {
      console.error("导出日志与配置失败:", error);
      return { success: false, error: error.message };
    }
  }

  static async getXrayVersion() {
    try {
      const result = await KSU.exec(`${KSU.MODULE_PATH}/bin/xray version`);
      const match = result.match(/Xray\s+([\d.]+)/);
      return match ? match[1] : "unknown";
    } catch (error) {
      console.error("Failed to get Xray version:", error);
      return "unknown";
    }
  }

  // 检查并更新 Xray 内核
  static updateXray() {
    return new Promise((resolve) => {
      let output = "";
      let resolved = false;
      const timeout = setTimeout(() => {
        if (!resolved) {
          resolved = true;
          resolve({
            success: false,
            isLatest: false,
            message: "更新超时",
            error: "操作超时",
          });
        }
      }, 60000);

      try {
        const sh = KSU.spawn("sh", [
          `${KSU.MODULE_PATH}/scripts/utils/update-xray.sh`,
        ]);

        sh.stdout.on("data", (data) => (output += data));
        sh.stderr.on("data", (data) => (output += data));

        sh.on("exit", (code) => {
          if (resolved) return;
          resolved = true;
          clearTimeout(timeout);

          if (code === 0) {
            if (
              output.includes("已是最新版本") ||
              output.includes("无需更新")
            ) {
              resolve({
                success: true,
                isLatest: true,
                message: "已是最新版本，无需更新",
                output,
              });
            } else if (
              output.includes("更新成功") ||
              output.includes("========== 更新成功")
            ) {
              resolve({
                success: true,
                isLatest: false,
                message: "更新成功",
                output,
              });
            } else {
              resolve({
                success: true,
                isLatest: false,
                message: "操作完成",
                output,
              });
            }
          } else {
            resolve({
              success: false,
              isLatest: false,
              message: "更新失败",
              error: output,
            });
          }
        });

        sh.on("error", (err) => {
          if (resolved) return;
          resolved = true;
          clearTimeout(timeout);
          resolve({
            success: false,
            isLatest: false,
            message: "更新失败",
            error: err.message,
          });
        });
      } catch (error: any) {
        if (!resolved) {
          resolved = true;
          clearTimeout(timeout);
          resolve({
            success: false,
            isLatest: false,
            message: "更新失败",
            error: error.message,
          });
        }
      }
    });
  }

  // 获取日志
  static async getServiceLog(lines = 100) {
    try {
      return await KSU.exec(
        `tail -n ${lines} ${KSU.MODULE_PATH}/logs/service.log`,
      );
    } catch (error) {
      return "暂无日志";
    }
  }

  static async getXrayLog(lines = 100) {
    try {
      return await KSU.exec(
        `tail -n ${lines} ${KSU.MODULE_PATH}/logs/xray.log`,
      );
    } catch (error) {
      return "暂无日志";
    }
  }

  static async renewTProxy() {
    try {
      await KSU.exec(`sh ${KSU.MODULE_PATH}/scripts/network/tproxy.sh restart`);
      return { success: true };
    } catch (error: any) {
      return { success: false, error: error.message };
    }
  }

  // ===================== Clash 规则导入 =====================

  /**
   * 获取 Clash 规则列表 URL 内容
   */
  static async fetchClashRules(url: string): Promise<string | null> {
    return KSU.fetchUrl(url);
  }

  /**
   * 解析 Clash YAML payload 格式
   * @param content YAML 内容
   * @returns 域名列表
   */
  static parseClashPayload(content: string): string[] {
    const domains: string[] = [];
    const lines = content.split("\n");
    let inPayload = false;

    for (const line of lines) {
      const trimmed = line.trim();

      // 检测 payload: 开始
      if (trimmed === "payload:") {
        inPayload = true;
        continue;
      }

      // 如果在 payload 区域，解析域名
      if (inPayload) {
        // 检测是否结束 (遇到非 - 开头的行且非空)
        if (trimmed && !trimmed.startsWith("-") && !trimmed.startsWith("#")) {
          break;
        }

        // 解析 - 'domain' 或 - "domain" 或 - domain 格式
        if (trimmed.startsWith("-")) {
          let domain = trimmed.substring(1).trim();
          // 移除引号
          domain = domain.replace(/^['"]|['"]$/g, "");
          // 移除 Clash 特殊前缀 (DOMAIN, DOMAIN-SUFFIX 等)
          domain = domain.replace(/^\+\.|^\*\./, "");

          if (domain && !domain.startsWith("#")) {
            domains.push(domain);
          }
        }
      }
    }

    return domains;
  }

  /**
   * 完整导入流程：获取 URL 内容并解析
   * @param url Clash 规则列表 URL
   * @returns 解析后的域名列表
   */
  static async importClashRulesFromUrl(url: string): Promise<string[]> {
    const content = await this.fetchClashRules(url);
    if (!content) {
      return [];
    }
    return this.parseClashPayload(content);
  }
}
