import { KSU } from "./ksu.js";

interface ConfigGroup {
  type: "default" | "subscription";
  name: string;
  dirName: string;
  configs: string[];
  url?: string;
  updated?: string;
}

interface Subscription {
  name: string;
  dirName: string;
  url?: string;
  updated?: string;
  nodeCount?: number;
}

interface ConfigInfo {
  protocol: string;
  address: string;
  port: string;
}

interface OperationResult {
  success: boolean;
  error?: string;
  output?: string;
}

/**
 * Config Service - 节点页面相关业务逻辑
 */
export class ConfigService {
  // ==================== 配置文件管理 ====================

  // 获取分组配置
  static async getConfigGroups(): Promise<ConfigGroup[]> {
    // 先获取默认分组
    const groups: ConfigGroup[] = [];
    const outboundsDir = `${KSU.MODULE_PATH}/config/xray/outbounds`;

    try {
      const defaultFiles = await KSU.exec(
        `find ${outboundsDir} -maxdepth 1 -name '*.json' -exec basename {} \\;`,
      );
      const defaultConfigs = defaultFiles.split("\n").filter((f) => f);
      if (defaultConfigs.length > 0) {
        groups.push({
          type: "default",
          name: "默认分组",
          dirName: "",
          configs: defaultConfigs,
        });
      }
    } catch (e) {}

    // 获取订阅分组
    const subscriptions = await this.getSubscriptions();
    for (const sub of subscriptions) {
      try {
        const files = await KSU.exec(
          `find ${outboundsDir}/${sub.dirName} -name '*.json' ! -name '_meta.json' -exec basename {} \\;`,
        );
        groups.push({
          type: "subscription",
          name: sub.name,
          dirName: sub.dirName,
          url: sub.url,
          updated: sub.updated,
          configs: files.split("\n").filter((f) => f),
        });
      } catch (e) {}
    }

    return groups;
  }

  // 读取配置文件（从 outbounds 目录）
  static async readConfig(filename: string): Promise<string> {
    return await KSU.exec(
      `cat '${KSU.MODULE_PATH}/config/xray/outbounds/${filename}'`,
    );
  }

  // 批量读取多个配置文件的基本信息
  static async batchReadConfigInfos(
    filePaths: string[],
  ): Promise<Map<string, ConfigInfo>> {
    if (!filePaths || filePaths.length === 0) return new Map();

    const basePath = `${KSU.MODULE_PATH}/config/xray/outbounds`;
    const fileList = filePaths.map((f) => `${basePath}/${f}`).join("\n");

    const result = await KSU.exec(`
            while IFS= read -r f; do
                [ -z "$f" ] && continue
                echo "===FILE:$(basename "$f")==="
                head -30 "$f" 2>/dev/null | grep -E '"protocol"|"address"|"port"' | head -5
            done << 'EOF'
${fileList}
EOF
        `);

    if (!result) return new Map();

    const infoMap = new Map<string, ConfigInfo>();
    const blocks = result.split("===FILE:").filter((b) => b.trim());

    for (const block of blocks) {
      const lines = block.split("\n");
      const filename = lines[0].replace("===", "").trim();
      const content = lines.slice(1).join("\n");

      let protocol = "unknown",
        address = "",
        port = "";
      const protocolMatch = content.match(/"protocol"\s*:\s*"([^"]+)"/);
      if (protocolMatch) protocol = protocolMatch[1];
      const addressMatch = content.match(/"address"\s*:\s*"([^"]+)"/);
      if (addressMatch) address = addressMatch[1];
      const portMatch = content.match(/"port"\s*:\s*(\d+)/);
      if (portMatch) port = portMatch[1];

      infoMap.set(filename, { protocol, address, port });
    }

    return infoMap;
  }

  // 保存配置文件
  static async saveConfig(filename: string, content: string): Promise<void> {
    const escaped = content.replace(/'/g, "'\\''");
    await KSU.exec(
      `echo '${escaped}' > '${KSU.MODULE_PATH}/config/xray/outbounds/${filename}'`,
    );
  }

  static async deleteConfig(filename: string): Promise<OperationResult> {
    try {
      await KSU.exec(
        `rm '${KSU.MODULE_PATH}/config/xray/outbounds/${filename}'`,
      );
      return { success: true };
    } catch (error: any) {
      return { success: false, error: error.message };
    }
  }

  // 切换配置（支持热切换）
  static async switchConfig(filename: string): Promise<void> {
    const configPath = `${KSU.MODULE_PATH}/config/xray/outbounds/${filename}`;

    // 需要检查服务状态来决定是热切换还是直接修改配置
    // 为了避免循环依赖，这里重复一下 pidof 检查，或者简单地都尝试调用 switch-config.sh
    // switch-config.sh 内部建议增加判断逻辑，目前 KSUService 逻辑是先检查状态
    const pidOutput = await KSU.exec(
      `pidof -s /data/adb/modules/netproxy/bin/xray 2>/dev/null || echo`,
    );
    const isRunning = pidOutput.trim() !== "";

    if (isRunning) {
      await KSU.exec(
        `sh ${KSU.MODULE_PATH}/scripts/core/switch-config.sh '${configPath}'`,
      );
    } else {
      await KSU.exec(
        `sed -i 's|^CURRENT_CONFIG=.*|CURRENT_CONFIG="${configPath}"|' ${KSU.MODULE_PATH}/config/module.conf`,
      );
    }
  }

  static async importFromNodeLink(nodeLink: string): Promise<OperationResult> {
    try {
      const escapedLink = nodeLink.replace(/'/g, "'\\''");
      const result = await KSU.exec(
        `cd '${KSU.MODULE_PATH}/config/xray/outbounds' && chmod +x '${KSU.MODULE_PATH}/bin/proxylink' && '${KSU.MODULE_PATH}/bin/proxylink' -parse '${escapedLink}' -insecure -format xray -auto`,
      );
      return { success: true, output: result };
    } catch (error: any) {
      console.error("Import from node link error:", error);
      return { success: false, error: error.message };
    }
  }

  // ==================== 订阅管理 ====================

  static async getSubscriptions(): Promise<Subscription[]> {
    try {
      const result = await KSU.exec(
        `find ${KSU.MODULE_PATH}/config/xray/outbounds -mindepth 1 -maxdepth 1 -type d -name 'sub_*' -exec basename {} \\;`,
      );
      const subscriptions: Subscription[] = [];

      for (const dir of result.split("\n").filter((d) => d)) {
        const name = dir.replace(/^sub_/, "");
        try {
          const metaContent = await KSU.exec(
            `cat ${KSU.MODULE_PATH}/config/xray/outbounds/${dir}/_meta.json`,
          );
          const meta = JSON.parse(metaContent);
          const nodeCount = await KSU.exec(
            `find ${KSU.MODULE_PATH}/config/xray/outbounds/${dir} -name '*.json' ! -name '_meta.json' | wc -l`,
          );
          subscriptions.push({
            name: meta.name || name,
            dirName: dir,
            url: meta.url,
            updated: meta.updated,
            nodeCount: parseInt(nodeCount.trim()) || 0,
          });
        } catch (e) {}
      }
      return subscriptions;
    } catch (error) {
      return [];
    }
  }

  static async addSubscription(
    name: string,
    url: string,
  ): Promise<OperationResult> {
    try {
      await KSU.exec(
        `sh ${KSU.MODULE_PATH}/scripts/config/subscription.sh add "${name}" "${url}"`,
      );
      return { success: true };
    } catch (error: any) {
      return { success: false, error: error.message };
    }
  }

  static async updateSubscription(name: string): Promise<OperationResult> {
    const statusFile = `${KSU.MODULE_PATH}/config/.sub_status`;
    await KSU.exec(`rm -f ${statusFile}`);
    // Fire-and-forget: spawn background script
    KSU.spawn("sh", [
      "-c",
      `sh ${KSU.MODULE_PATH}/scripts/config/subscription.sh update "${name}" && echo success > ${statusFile} || echo fail > ${statusFile}`,
    ]);
    return await this.waitForSubscriptionComplete(statusFile, 60000);
  }

  static async removeSubscription(name: string): Promise<OperationResult> {
    try {
      await KSU.exec(
        `sh ${KSU.MODULE_PATH}/scripts/config/subscription.sh remove '${name}'`,
      );
      return { success: true };
    } catch (error: any) {
      throw new Error(error.message || "删除订阅失败");
    }
  }

  static async waitForSubscriptionComplete(
    statusFile: string,
    timeout: number,
  ): Promise<OperationResult> {
    const startTime = Date.now();
    const pollInterval = 500;

    while (Date.now() - startTime < timeout) {
      await new Promise((resolve) => setTimeout(resolve, pollInterval));
      try {
        const status = await KSU.exec(
          `cat ${statusFile} 2>/dev/null || echo ""`,
        );

        if (status.trim() === "success") {
          await KSU.exec(`rm -f ${statusFile}`);
          return { success: true };
        } else if (status.trim() === "fail") {
          await KSU.exec(`rm -f ${statusFile}`);
          throw new Error("订阅操作失败");
        }
      } catch (e) {
        // Continue polling
      }
    }
    throw new Error("操作超时");
  }
}
