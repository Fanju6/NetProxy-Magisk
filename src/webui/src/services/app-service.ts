import { KSU } from "./ksu.js";

interface UserInfo {
  id: string;
  name: string;
}

interface AppInfo {
  packageName: string;
  userId: string;
  appLabel?: string;
  icon?: string | null;
}

interface ProxyAppConfig {
  userId: string;
  packageName: string;
}

/**
 * App Service - 应用页面 (UidPage) 相关业务逻辑
 */
export class AppService {
  // 获取分应用代理模式
  static async getAppProxyMode(): Promise<string> {
    try {
      const content = await KSU.exec(
        `cat ${KSU.MODULE_PATH}/config/tproxy.conf`,
      );
      const match = content.match(/^APP_PROXY_MODE="?(\w+)"?/m);
      return match ? match[1] : "blacklist";
    } catch (error) {
      return "blacklist";
    }
  }

  // 设置分应用代理模式
  static async setAppProxyMode(mode: string): Promise<void> {
    await KSU.exec(
      `sed -i 's/^APP_PROXY_MODE=.*/APP_PROXY_MODE="${mode}"/' ${KSU.MODULE_PATH}/config/tproxy.conf`,
    );
  }

  // 获取分应用代理启用状态
  static async getAppProxyEnabled(): Promise<boolean> {
    try {
      const content = await KSU.exec(
        `cat ${KSU.MODULE_PATH}/config/tproxy.conf`,
      );
      // APP_PROXY_ENABLE=1 or 0
      const match = content.match(/^APP_PROXY_ENABLE=(\d+)/m);
      return match ? match[1] === "1" : true; // 默认启用? 假设
    } catch (error) {
      return true;
    }
  }

  // 设置分应用代理启用状态
  static async setAppProxyEnabled(enabled: boolean): Promise<void> {
    const val = enabled ? "1" : "0";
    await KSU.exec(
      `sed -i 's/^APP_PROXY_ENABLE=.*/APP_PROXY_ENABLE=${val}/' ${KSU.MODULE_PATH}/config/tproxy.conf`,
    );
  }

  // 获取已安装应用列表
  static async getUsers(): Promise<UserInfo[]> {
    try {
      const output = await KSU.exec("pm list users");
      const users: UserInfo[] = [];
      // Output format:
      // Users:
      // 	UserInfo{0:Owner:13} running
      // 	UserInfo{999:MultiApp:4001010} running
      const lines = output.split("\n");
      for (const line of lines) {
        const match = line.match(/UserInfo\{(\d+):([^:]+):/);
        if (match) {
          users.push({
            id: match[1],
            name: match[2],
          });
        }
      }
      return users;
    } catch (error) {
      console.error("Failed to get users:", error);
      return [{ id: "0", name: "Owner" }];
    }
  }

  static async getInstalledApps(
    userId = "0",
    showSystem = false,
  ): Promise<AppInfo[]> {
    // 构建 pm options
    // if showSystem is true -> list packages -s (仅系统)
    // if showSystem is false -> list packages -3 (仅第三方)

    const filter = showSystem ? "-s" : "-3";
    const cmd = `pm list packages --user ${userId} ${filter}`;

    try {
      const output = await KSU.exec(cmd);
      const lines = output.split("\n");
      const apps: AppInfo[] = [];

      for (const line of lines) {
        // package:com.android.chrome
        const packageName = line.replace(/^package:/, "").trim();
        if (!packageName) continue;

        apps.push({
          packageName: packageName,
          appLabel: packageName, // 懒加载 Label
          userId: userId,
          icon: null, // 懒加载 Icon
        });
      }

      return apps;
    } catch (error) {
      console.error("Failed to get installed apps:", error);
      return [];
    }
  }

  // 获取应用详情（Label, Icon），失败或返回空时自动重试
  static async fetchAppDetails(
    apps: AppInfo[],
    retries = 10,
  ): Promise<AppInfo[]> {
    const uniquePkgs = [...new Set(apps.map((a) => a.packageName))];
    if (uniquePkgs.length === 0) return apps;

    for (let attempt = 0; attempt < retries; attempt++) {
      try {
        const infos = KSU.getPackagesInfo(uniquePkgs);

        // 检查是否获取到有效数据
        if (!infos || infos.length === 0) {
          console.warn(
            `fetchAppDetails attempt ${attempt + 1}: empty result, retrying...`,
          );
          await new Promise((resolve) => setTimeout(resolve, 300));
          continue;
        }

        const infoMap = new Map();
        infos.forEach((i: any) => infoMap.set(i.packageName, i));

        apps.forEach((app) => {
          const info = infoMap.get(app.packageName);
          if (info) {
            app.appLabel = info.appLabel || app.appLabel;
            app.icon = `ksu://icon/${app.packageName}`;
          }
        });

        // 检查是否至少有一个应用获取到了 appLabel
        const hasLabels = apps.some(
          (app) => app.appLabel && app.appLabel !== app.packageName,
        );
        if (hasLabels) {
          return apps; // 成功获取
        }

        console.warn(
          `fetchAppDetails attempt ${attempt + 1}: no labels populated, retrying...`,
        );
        await new Promise((resolve) => setTimeout(resolve, 300));
      } catch (e) {
        console.warn(`fetchAppDetails attempt ${attempt + 1} failed:`, e);
        if (attempt < retries - 1) {
          await new Promise((resolve) => setTimeout(resolve, 300));
        }
      }
    }

    return apps;
  }

  // 获取代理应用列表
  static async getProxyApps(): Promise<ProxyAppConfig[]> {
    try {
      const content = await KSU.exec(
        `cat ${KSU.MODULE_PATH}/config/tproxy.conf`,
      );
      const mode =
        (content.match(/APP_PROXY_MODE="?(\w+)"?/) || [])[1] || "blacklist";

      // 黑名单模式用 BYPASS_APPS_LIST，白名单模式用 PROXY_APPS_LIST
      const listKey =
        mode === "blacklist" ? "BYPASS_APPS_LIST" : "PROXY_APPS_LIST";
      const match = content.match(new RegExp(`${listKey}="([^"]*)"`));

      if (match && match[1]) {
        return match[1]
          .split(" ")
          .filter((item) => item.trim())
          .map((item) => {
            const parts = item.split(":");
            if (parts.length === 2) {
              return { userId: parts[0], packageName: parts[1] };
            }
            // 兼容旧格式 (纯包名)，默认为主用户 0
            return { userId: "0", packageName: item };
          });
      }
      return [];
    } catch (error) {
      return [];
    }
  }

  // 添加代理应用
  static async addProxyApp(packageName: string, userId = "0"): Promise<void> {
    const content = await KSU.exec(`cat ${KSU.MODULE_PATH}/config/tproxy.conf`);
    const mode =
      (content.match(/APP_PROXY_MODE="?(\w+)"?/) || [])[1] || "blacklist";
    const listKey =
      mode === "blacklist" ? "BYPASS_APPS_LIST" : "PROXY_APPS_LIST";
    const match = content.match(new RegExp(`${listKey}="([^"]*)"`));
    const currentListStr = match ? match[1] : "";
    const currentList = currentListStr.split(" ").filter((i) => i.trim());

    const newItem = `${userId}:${packageName}`;
    if (currentList.includes(newItem)) {
      return; // 已存在
    }

    const newList =
      currentList.length > 0 ? `${currentListStr} ${newItem}` : newItem;
    await KSU.exec(
      `sed -i 's/${listKey}="[^"]*"/${listKey}="${newList}"/' ${KSU.MODULE_PATH}/config/tproxy.conf`,
    );
  }

  // 删除代理应用
  static async removeProxyApp(
    packageName: string,
    userId = "0",
  ): Promise<void> {
    const content = await KSU.exec(`cat ${KSU.MODULE_PATH}/config/tproxy.conf`);
    const mode =
      (content.match(/APP_PROXY_MODE="?(\w+)"?/) || [])[1] || "blacklist";
    const listKey =
      mode === "blacklist" ? "BYPASS_APPS_LIST" : "PROXY_APPS_LIST";
    const match = content.match(new RegExp(`${listKey}="([^"]*)"`));
    const currentListStr = match ? match[1] : "";
    const currentList = currentListStr.split(" ").filter((i) => i.trim());

    const targetItem = `${userId}:${packageName}`;
    // 过滤掉完全匹配的项
    const newList = currentList
      .filter((item) => {
        // 兼容处理：如果列表中是纯包名 (旧数据)，我们也尝试匹配
        if (item === packageName && userId === "0") return false;
        return item !== targetItem;
      })
      .join(" ");

    await KSU.exec(
      `sed -i 's/${listKey}="[^"]*"/${listKey}="${newList}"/' ${KSU.MODULE_PATH}/config/tproxy.conf`,
    );
  }
}
