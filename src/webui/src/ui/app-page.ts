import { AppService } from "../services/app-service.js";
import { toast } from "../utils/toast.js";
import { I18nService } from "../i18n/i18n-service.js";
import type { UI } from "./ui-core.js";

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
  appLabel?: string;
  icon?: string | null;
}

/**
 * 代理设置页面管理器
 */
export class AppPageManager {
  ui: UI;
  allApps: AppInfo[];
  proxyMode: string;
  proxyApps: ProxyAppConfig[];
  selectedApps: Map<string, AppInfo>;
  originalProxyApps: Map<string, AppInfo>;
  users: UserInfo[];
  currentUserId: string;
  showSystemApps: boolean;
  appProxyEnabled: boolean;
  isUpdatingUI: boolean;

  constructor(ui: UI) {
    this.ui = ui;
    this.allApps = [];
    this.proxyMode = "blacklist";
    this.proxyApps = [];
    this.selectedApps = new Map();
    this.originalProxyApps = new Map();
    this.users = [];
    this.currentUserId = "0";
    this.showSystemApps = false; // 默认不显示系统应用 (-3 only)
    this.appProxyEnabled = true; // 分应用代理总开关
    this.isUpdatingUI = false; // 防止程序更新 UI 触发事件
  }

  async init(): Promise<void> {
    // 绑定分应用设置（三态：关闭/白名单/黑名单）
    const modeGroup = document.getElementById("app-proxy-mode-group") as any;
    if (modeGroup) {
      modeGroup.addEventListener("change", async () => {
        if (this.isUpdatingUI) return;
        const newMode = modeGroup.value;
        await this.handleProxyModeChange(newMode);
      });
    }

    // 初始化用户列表（在打开 Dialog 时刷新，这里先不加载）

    // 绑定 Dialog 内的过滤器事件
    const filterInput = document.getElementById(
      "app-selector-search",
    ) as HTMLInputElement | null;
    if (filterInput) {
      filterInput.addEventListener("input", () =>
        this.filterApps(filterInput.value),
      );
    }

    const userSelect = document.getElementById("app-selector-user") as any;
    if (userSelect) {
      userSelect.addEventListener("change", () => {
        this.currentUserId = userSelect.value;
        this.reloadAppList();
      });
    }

    const systemSwitch = document.getElementById(
      "app-selector-show-system",
    ) as HTMLInputElement | null;
    if (systemSwitch) {
      systemSwitch.addEventListener("change", () => {
        this.showSystemApps = systemSwitch.checked;
        this.reloadAppList();
      });
    }
  }

  async update(forceRefresh = false) {
    try {
      const listEl = document.getElementById("uid-list");
      const modeSwitch = document.getElementById("proxy-mode-switch");
      const modeDesc = document.getElementById("proxy-mode-desc");
      const listTitle = document.getElementById("proxy-list-title");

      // 如果已有缓存且不是强制刷新，不做任何操作（DOM 已正确渲染）
      if (!forceRefresh && this.proxyApps.length > 0) {
        return;
      }

      if (!listEl) return;

      // 显示骨架屏
      const currentCount =
        listEl.children.length > 0 ? listEl.children.length : 1;
      this.ui.showSkeleton(listEl, currentCount);

      // 获取代理模式和开关状态
      const [mode, enabled] = await Promise.all([
        AppService.getAppProxyMode(),
        AppService.getAppProxyEnabled(),
      ]);
      this.proxyMode = mode;
      this.appProxyEnabled = enabled;

      // 计算当前的三态值
      let currentModeValue = "off";
      if (this.appProxyEnabled) {
        currentModeValue = this.proxyMode; // 'blacklist' or 'whitelist'
      }

      // 更新 segmented button (使用标志位防止触发 change 事件)
      const modeGroup = document.getElementById("app-proxy-mode-group") as any;
      if (modeGroup) {
        this.isUpdatingUI = true;
        modeGroup.value = currentModeValue;
        // mdui 可能异步触发 change 事件，延迟重置标志位
        setTimeout(() => {
          this.isUpdatingUI = false;
        }, 100);
      }

      // 控制列表显示/隐藏
      const listCard = document.getElementById("proxy-list-card");
      if (listCard) {
        if (currentModeValue === "off") {
          listCard.style.display = "none";
        } else {
          listCard.style.display = "block";
        }
      }

      // 更新列表标题
      if (listTitle && currentModeValue !== "off") {
        listTitle.textContent =
          this.proxyMode === "blacklist"
            ? I18nService.t("uid.title_blacklist")
            : I18nService.t("uid.title_whitelist");
      }

      // 获取代理应用列表 Array<{userId, packageName}>
      this.proxyApps = await AppService.getProxyApps();

      if (this.proxyApps.length === 0) {
        const emptyText =
          this.proxyMode === "blacklist"
            ? I18nService.t("uid.empty_blacklist")
            : I18nService.t("uid.empty_whitelist");
        listEl.innerHTML = `<mdui-list-item><div slot="headline">${emptyText}</div></mdui-list-item>`;
        return;
      }

      // 获取所有应用信息（Label, Icon）- 重试逻辑已内置于 fetchAppDetails
      this.proxyApps = await AppService.fetchAppDetails(this.proxyApps);

      // 列表显示时，如果有 cache 则显示 Label，否则显示 PackageName。

      // 图标懒加载观察器
      const observer = new IntersectionObserver(
        (entries) => {
          entries.forEach((entry) => {
            if (entry.isIntersecting) {
              const item = entry.target;
              const img = item.querySelector(
                "img.app-icon",
              ) as HTMLImageElement | null;
              if (img && !img.src) {
                const iconUrl = img.dataset.iconUrl;
                const packageName = img.dataset.packageName;
                const userId = img.dataset.userId || "0";

                if (iconUrl) {
                  img.src = iconUrl;
                  img.onload = () => {
                    img.style.display = "block";
                    const placeholder = item.querySelector(
                      'mdui-icon[slot="icon"]',
                    ) as HTMLElement | null;
                    if (placeholder) placeholder.style.display = "none";
                  };
                } else if (packageName) {
                  // 使用 KSU API 图标 URL
                  img.src = `ksu://icon/${packageName}`;
                  img.onload = () => {
                    img.style.display = "block";
                    const placeholder = item.querySelector(
                      'mdui-icon[slot="icon"]',
                    ) as HTMLElement | null;
                    if (placeholder) placeholder.style.display = "none";
                  };
                }
              }
              observer.unobserve(item);
            }
          });
        },
        {
          rootMargin: "100px",
          threshold: 0.1,
        },
      );

      listEl.innerHTML = "";
      // this.proxyApps is Array<{userId, packageName}>
      this.proxyApps.forEach((proxyApp) => {
        const item = document.createElement("mdui-list-item");
        // 使用 fetchAppDetails 获取到的 Label，如果没有则显示包名
        const label = proxyApp.appLabel || proxyApp.packageName;

        // 标题：应用名 + 用户ID（如果非主用户）
        const headline =
          proxyApp.userId !== "0"
            ? `${label} [${I18nService.t("uid.user_label")} ${proxyApp.userId}]`
            : label;
        item.setAttribute("headline", headline);

        // Description 只显示包名（如果和标签不同）
        if (label !== proxyApp.packageName) {
          const descSpan = document.createElement("span");
          descSpan.slot = "description";
          descSpan.className = "package-name-wrap";
          descSpan.textContent = proxyApp.packageName;
          item.appendChild(descSpan);
        }

        // 统一使用懒加载方式
        const icon = document.createElement("mdui-icon");
        icon.slot = "icon";
        icon.setAttribute("name", "android");
        item.appendChild(icon);

        const iconEl = document.createElement("img");
        iconEl.slot = "icon";
        iconEl.className = "app-icon";
        iconEl.style.display = "none";

        // 设置 dataset 供 IntersectionObserver 使用
        iconEl.dataset.packageName = proxyApp.packageName;
        iconEl.dataset.userId = proxyApp.userId;

        // 如果 fetchAppDetails 已经获取到了 icon (例如 ksu://), 直接设置 dataset.iconUrl
        if (proxyApp.icon) {
          iconEl.dataset.iconUrl = proxyApp.icon;
        }

        iconEl.onerror = function (this: HTMLImageElement) {
          this.style.display = "none";
          const placeholder = this.parentElement?.querySelector(
            'mdui-icon[slot="icon"]',
          ) as HTMLElement | null;
          if (placeholder) placeholder.style.display = "";
        };

        item.appendChild(iconEl);
        observer.observe(item);

        // 添加删除按钮
        const deleteBtn = document.createElement("mdui-button-icon");
        deleteBtn.slot = "end-icon";
        deleteBtn.setAttribute("icon", "delete");
        deleteBtn.style.color = "var(--mdui-color-error)";
        deleteBtn.addEventListener("click", async (e) => {
          e.stopPropagation();
          // 传递 appLabel (如果已获取) 或 default to packageName
          await this.removeApp(proxyApp.packageName, proxyApp.userId, label);
        });
        item.appendChild(deleteBtn);

        listEl.appendChild(item);
      });
    } catch (error) {
      console.error("Update proxy page failed:", error);
    }
  }

  async handleProxyModeChange(modeValue: string): Promise<void> {
    try {
      if (modeValue === "off") {
        // 关闭功能
        await AppService.setAppProxyEnabled(false);
        this.appProxyEnabled = false;
        toast(I18nService.t("uid.toast_proxy_disabled"));
      } else {
        // 开启功能并设置模式
        await AppService.setAppProxyEnabled(true);
        await AppService.setAppProxyMode(modeValue);
        this.appProxyEnabled = true;
        this.proxyMode = modeValue;
        toast(
          I18nService.t("uid.toast_mode_switched") +
            (modeValue === "blacklist"
              ? I18nService.t("uid.mode_blacklist")
              : I18nService.t("uid.mode_whitelist")),
        );
      }
      this.update(true);
    } catch (error: any) {
      toast(I18nService.t("common.set_failed") + error.message, true);
    }
  }

  async removeApp(
    packageName: string,
    userId: string = "0",
    appLabel: string | null = null,
  ): Promise<void> {
    const userIdStr = userId.toString();
    const displayUser = userIdStr !== "0" ? `(用户 ${userIdStr})` : "";
    const displayName = appLabel || packageName;
    if (
      await this.ui.confirm(
        I18nService.t("uid.confirm_remove", {
          name: displayName + (displayUser ? " " + displayUser : ""),
        }),
      )
    ) {
      try {
        await AppService.removeProxyApp(packageName, userIdStr);
        toast(I18nService.t("uid.toast_removed"));
        // 清除缓存并强制刷新
        this.proxyApps = [];
        await this.update(true);
      } catch (error: any) {
        toast(I18nService.t("uid.toast_remove_failed") + error.message, true);
      }
    }
  }

  async reloadAppList(): Promise<void> {
    const listEl = document.getElementById("app-selector-list");
    if (!listEl) return;
    // 骨架屏
    this.ui.showSkeleton(listEl, 5);
    try {
      // 重新获取 (带缓存或者重新 exec)
      this.allApps = await AppService.getInstalledApps(
        this.currentUserId,
        this.showSystemApps,
      );
      // 获取应用详情（Label, Icon）
      this.allApps = await AppService.fetchAppDetails(this.allApps);
      this.renderAppList(this.allApps);
    } catch (error: any) {
      listEl.innerHTML = `<mdui-list-item><div slot="headline">${I18nService.t("logs.load_failed")}</div></mdui-list-item>`;
      toast(I18nService.t("uid.toast_load_apps_failed") + error.message, true);
    }
  }

  async showAppSelector(): Promise<void> {
    const dialog = document.getElementById("app-selector-dialog") as any;
    const listEl = document.getElementById("app-selector-list");
    const addSelectedBtn = document.getElementById("app-selector-add-selected");

    // 清空搜索输入框并重置筛选状态
    const filterInput = document.getElementById(
      "app-selector-search",
    ) as HTMLInputElement | null;
    if (filterInput) {
      filterInput.value = "";
    }

    // 加载用户列表
    const userSelect = document.getElementById("app-selector-user") as any;
    if (userSelect) {
      this.users = await AppService.getUsers();
      userSelect.innerHTML = "";
      this.users.forEach((u) => {
        const opt = document.createElement("mdui-menu-item") as any; // 或者 mdui-option, 取决于 select 实现
        // mdui-select 使用 mdui-menu-item
        opt.value = u.id;
        opt.textContent = `${u.name} (${u.id})`;
        userSelect.appendChild(opt);
      });
      userSelect.value = this.currentUserId;
    }

    // 清空选中状态
    this.selectedApps.clear();
    this.originalProxyApps.clear();

    // 预填充已在代理列表中的应用
    for (const proxyApp of this.proxyApps) {
      const key = `${proxyApp.userId}:${proxyApp.packageName}`;
      const appInfo: AppInfo = {
        packageName: proxyApp.packageName,
        userId: proxyApp.userId,
        appLabel: proxyApp.appLabel,
        icon: proxyApp.icon,
      };
      this.selectedApps.set(key, appInfo);
      this.originalProxyApps.set(key, appInfo);
    }

    this.updateAddSelectedButton();

    // 绑定批量添加按钮事件（每次打开都重新绑定）
    if (addSelectedBtn) {
      addSelectedBtn.onclick = () => this.addSelectedApps();
    }

    if (dialog) dialog.open = true;

    this.reloadAppList();
  }

  updateAddSelectedButton(): void {
    const btn = document.getElementById("app-selector-add-selected") as any;
    if (btn) {
      const count = this.selectedApps.size;
      btn.textContent = I18nService.t("uid.btn_add_selected", {
        count: String(count),
      });
      const hasChanges =
        count > 0 || this.originalProxyApps.size !== this.selectedApps.size;
      btn.disabled = !hasChanges;
    }
  }

  toggleAppSelection(
    app: AppInfo,
    checkbox: any,
    fromCheckbox: boolean = false,
  ): void {
    const key = `${app.userId}:${app.packageName}`;
    if (fromCheckbox) {
      // 从复选框触发：复选框已经自动切换了状态，直接根据当前状态更新数据
      if (checkbox.checked) {
        this.selectedApps.set(key, app);
      } else {
        this.selectedApps.delete(key);
      }
    } else {
      // 从列表项触发：需要手动切换复选框状态
      if (this.selectedApps.has(key)) {
        this.selectedApps.delete(key);
        checkbox.checked = false;
      } else {
        this.selectedApps.set(key, app);
        checkbox.checked = true;
      }
    }
    this.updateAddSelectedButton();
  }

  async addSelectedApps(): Promise<void> {
    // 找出需要移除的应用（在原始列表中但不在选中列表中）
    const appsToRemove: AppInfo[] = [];
    for (const [key, app] of this.originalProxyApps) {
      if (!this.selectedApps.has(key)) {
        appsToRemove.push(app);
      }
    }

    // 找出需要添加的应用
    const appsToAdd = Array.from(this.selectedApps.values());

    // 移除被取消勾选的应用
    for (const app of appsToRemove) {
      try {
        await AppService.removeProxyApp(app.packageName, app.userId);
      } catch (error) {
        // 忽略错误
      }
    }

    // 添加新选中的应用
    for (const app of appsToAdd) {
      try {
        await AppService.addProxyApp(app.packageName, app.userId);
      } catch (error) {
        // 忽略错误（如应用已存在）
      }
    }

    // 根据操作显示提示
    if (appsToRemove.length > 0 && appsToAdd.length > 0) {
      toast(
        I18nService.t("uid.toast_added_count", {
          count: String(appsToAdd.length),
        }),
      );
    } else if (appsToRemove.length > 0) {
      toast(I18nService.t("uid.toast_removed"));
    } else if (appsToAdd.length > 0) {
      toast(
        I18nService.t("uid.toast_added_count", {
          count: String(appsToAdd.length),
        }),
      );
    }

    // 关闭对话框并刷新列表
    const dialog = document.getElementById("app-selector-dialog") as any;
    if (dialog) dialog.open = false;
    this.selectedApps.clear();
    this.originalProxyApps.clear();
    this.proxyApps = [];
    await this.update(true);
  }

  renderAppList(apps: AppInfo[]): void {
    const listEl = document.getElementById("app-selector-list");
    if (!listEl) return;
    // 获取滚动容器作为 IntersectionObserver 的 root
    const scrollContainer = listEl.parentElement;

    if (apps.length === 0) {
      listEl.innerHTML = `<mdui-list-item><div slot="headline">${I18nService.t("uid.no_apps_found")}</div></mdui-list-item>`;
      return;
    }

    listEl.innerHTML = "";

    const observer = new IntersectionObserver(
      (entries) => {
        entries.forEach((entry) => {
          if (entry.isIntersecting) {
            const item = entry.target;
            const img = item.querySelector(
              "img.app-icon",
            ) as HTMLImageElement | null;
            if (img && !img.src) {
              const iconUrl = img.dataset.iconUrl;
              const packageName = img.dataset.packageName;

              if (iconUrl) {
                // 使用 ksu://icon/ URL
                img.src = iconUrl;
                img.onload = () => {
                  img.style.display = "block";
                  const placeholder = item.querySelector(
                    'mdui-icon[slot="icon"]',
                  ) as HTMLElement | null;
                  if (placeholder) {
                    placeholder.style.display = "none";
                  }
                };
              } else if (packageName) {
                // 使用 KSU API 图标 URL
                img.src = `ksu://icon/${packageName}`;
                img.onload = () => {
                  img.style.display = "block";
                  const placeholder = item.querySelector(
                    'mdui-icon[slot="icon"]',
                  ) as HTMLElement | null;
                  if (placeholder) {
                    placeholder.style.display = "none";
                  }
                };
              }
            }
            observer.unobserve(item);
          }
        });
      },
      {
        root: scrollContainer,
        rootMargin: "50px",
        threshold: 0.1,
      },
    );

    apps.forEach((app) => {
      const item = document.createElement("mdui-list-item");
      item.setAttribute("clickable", "");
      item.setAttribute("headline", app.appLabel || app.packageName);
      item.setAttribute("description", app.packageName);

      // 添加应用图标 - 统一使用懒加载
      const icon = document.createElement("mdui-icon");
      icon.slot = "icon";
      icon.setAttribute("name", "android");
      item.appendChild(icon);

      const iconEl = document.createElement("img");
      iconEl.slot = "icon";
      iconEl.className = "app-icon";
      iconEl.style.display = "none";

      if (app.icon) {
        // KSU API 方式：有 ksu://icon/ URL，也使用懒加载
        iconEl.dataset.iconUrl = app.icon;
      }
      // WebUI X 方式：通过包名懒加载
      iconEl.dataset.packageName = app.packageName;

      iconEl.onerror = function (this: HTMLImageElement) {
        this.style.display = "none";
        const placeholder = this.parentElement?.querySelector(
          'mdui-icon[slot="icon"]',
        ) as HTMLElement | null;
        if (placeholder) {
          placeholder.style.display = "";
        }
      };

      item.appendChild(iconEl);
      observer.observe(item);

      // 添加复选框
      const checkbox = document.createElement("mdui-checkbox") as any;
      checkbox.slot = "end-icon";
      const key = `${app.userId}:${app.packageName}`;
      checkbox.checked = this.selectedApps.has(key);
      item.appendChild(checkbox);

      // 复选框变化事件 - 阻止冒泡并同步选中状态
      checkbox.addEventListener("change", (e: Event) => {
        e.stopPropagation();
        this.toggleAppSelection(app, checkbox, true);
      });

      // 点击整行切换选中状态（排除复选框区域）
      item.addEventListener("click", (e) => {
        // 如果点击的是复选框本身，不处理（让 change 事件处理）
        if (e.target === checkbox || checkbox.contains(e.target as Node)) {
          return;
        }
        e.stopPropagation();
        this.toggleAppSelection(app, checkbox, false);
      });

      listEl.appendChild(item);
    });
  }

  filterApps(query: string): void {
    if (!this.allApps) return;

    const filtered = this.allApps.filter(
      (app) =>
        (app.appLabel || "").toLowerCase().includes(query.toLowerCase()) ||
        app.packageName.toLowerCase().includes(query.toLowerCase()),
    );

    this.renderAppList(filtered);
  }
}
