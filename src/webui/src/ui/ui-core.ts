import { setTheme } from "mdui";
import { StatusService } from "../services/status-service.js";
import { SettingsService } from "../services/settings-service.js";
import { I18nService } from "../i18n/i18n-service.js";
import { toast } from "../utils/toast.js";
import { StatusPageManager } from "./status-page.js";
import { ConfigPageManager } from "./config-page.js";
import { AppPageManager } from "./app-page.js";
import { SettingsPageManager } from "./settings-page.js";

interface SkeletonOptions {
  showIcon?: boolean;
}

/**
 * UI 核心管理器
 */
export class UI {
  currentPage: string;
  currentTheme: string;
  statusPage: StatusPageManager;
  configPage: ConfigPageManager;
  appPage: AppPageManager;
  settingsPage: SettingsPageManager;

  constructor() {
    this.currentPage = "status";
    // 从localStorage读取主题，如果不存在则使用auto
    this.currentTheme = localStorage.getItem("theme") || "auto";

    // 初始化页面管理器
    this.statusPage = new StatusPageManager(this);
    this.configPage = new ConfigPageManager(this);
    this.appPage = new AppPageManager(this);
    this.settingsPage = new SettingsPageManager(this);

    // 立即应用主题，避免闪烁
    this.applyTheme(this.currentTheme);

    this.init();
  }

  init(): void {
    // 初始化多语言服务
    I18nService.init();

    if (document.readyState === "loading") {
      document.addEventListener("DOMContentLoaded", () => {
        this.initializeMDUI();
      });
    } else {
      this.initializeMDUI();
    }

    this.setupNavigation();
    this.setupFAB();
    this.setupThemeToggle();
    this.setupDialogs();
    this.appPage.init();
    this.setupAppSelector();
    this.settingsPage.init();
    this.statusPage.setupModeButtons();

    // 初始化页面状态 (包括按钮可见性)
    this.switchPage(this.currentPage);

    try {
      // this.updateAllPages(); // switchPage 会调用 update
    } catch (error) {
      console.error("ERROR calling updateAllPages():", error);
    }

    setInterval(() => {
      const statusPage = document.getElementById("status-page");
      if (statusPage && statusPage.classList.contains("active")) {
        this.statusPage.update();
      }
    }, 9000);
  }

  initializeMDUI(): void {
    const requiredComponents = [
      "mdui-layout",
      "mdui-top-app-bar",
      "mdui-card",
      "mdui-button",
    ];
    requiredComponents.forEach((component) => {
      if (!customElements.get(component)) {
        console.warn(`⚠️ Component ${component} is not defined yet`);
      }
    });
  }

  setupNavigation(): void {
    const navBar = document.getElementById("nav-bar");
    if (navBar) {
      navBar.addEventListener("change", (e: any) => {
        const pageName = e.target.value;
        this.switchPage(pageName);
      });
    }

    const editDashboardBtn = document.getElementById("edit-dashboard-btn");
    if (editDashboardBtn) {
      editDashboardBtn.addEventListener("click", () => {
        this.statusPage.toggleEditMode();
      });
    }
  }

  switchPage(pageName: string): void {
    // 如果离开日志页面，停止自动刷新
    if (this.currentPage === "logs" && pageName !== "logs") {
      this.settingsPage.onLogsPageLeave();
    }

    document.querySelectorAll(".page").forEach((page) => {
      page.classList.remove("active");
    });
    const targetPage = document.getElementById(`${pageName}-page`);
    if (targetPage) {
      targetPage.classList.add("active");
    }
    this.currentPage = pageName;

    // 更新 FAB 可见性
    const fabContainer = document.getElementById("dashboard-fab");
    if (fabContainer) {
      fabContainer.style.display = pageName === "status" ? "block" : "none";
    }

    const editBtn = document.getElementById("edit-dashboard-btn");
    if (editBtn) {
      editBtn.style.display = pageName === "status" ? "block" : "none";
      // 如果切出状态页，确保退出编辑模式
      if (pageName !== "status" && this.statusPage.isEditing) {
        this.statusPage.toggleEditMode();
      }
    }

    // 延迟执行更新，让导航栏动画完全完成
    // MDUI 导航栏动画大约需要 200ms 完成
    setTimeout(() => {
      if (pageName === "status") this.statusPage.update();
      if (pageName === "config") this.configPage.update();
      if (pageName === "uid") this.appPage.update();
      if (pageName === "settings") this.settingsPage.update();
      if (pageName === "logs") this.settingsPage.updateLogs();
    }, 200);
  }

  setupFAB(): void {
    const fab = document.getElementById("service-fab") as any;
    if (!fab) {
      console.warn("FAB element not found");
      return;
    }

    fab.addEventListener("click", async (e: MouseEvent) => {
      e.stopPropagation(); // 防止事件冒泡

      // 防止重复点击
      if (fab.loading) return;
      fab.loading = true;

      try {
        // 如果当前显示为运行中（有 running 类），则意图是停止；否则是启动
        const fabContainer = document.getElementById("dashboard-fab");
        const isRunning =
          fabContainer && fabContainer.classList.contains("running");

        // 显示加载状态
        fab.icon = "hourglass_empty";

        if (isRunning) {
          const success = await StatusService.stopService();
          if (!success) {
            toast(I18nService.t("status.service_stop_timeout"));
          }
        } else {
          const success = await StatusService.startService();
          if (!success) {
            toast(I18nService.t("status.service_start_timeout"));
          }
        }

        await this.statusPage.update();
      } catch (error: any) {
        console.error("FAB error:", error);
        toast(I18nService.t("common.operation_failed") + error.message);
      } finally {
        fab.loading = false;
      }
    });
  }

  setupThemeToggle(): void {
    const themeBtn = document.getElementById("theme-toggle");
    this.applyTheme(this.currentTheme);

    if (themeBtn) {
      themeBtn.addEventListener("click", () => {
        const themes = ["light", "dark", "auto"];
        const currentIndex = themes.indexOf(this.currentTheme);
        this.currentTheme = themes[(currentIndex + 1) % themes.length];
        localStorage.setItem("theme", this.currentTheme);
        this.applyTheme(this.currentTheme);
        const modeName =
          this.currentTheme === "auto"
            ? I18nService.t("settings.theme.mode_auto")
            : this.currentTheme === "light"
              ? I18nService.t("settings.theme.mode_light")
              : I18nService.t("settings.theme.mode_dark");
        toast(I18nService.t("settings.theme.toast_mode_switched") + modeName);
      });
    }
  }

  applyTheme(theme: string): void {
    const html = document.documentElement;
    const savedMonet = localStorage.getItem("monetEnabled");
    const isDark =
      window.matchMedia &&
      window.matchMedia("(prefers-color-scheme: dark)").matches;

    // 移除所有主题类
    html.classList.remove(
      "mdui-theme-light",
      "mdui-theme-dark",
      "mdui-theme-auto",
    );

    if (theme === "light") {
      html.classList.add("mdui-theme-light");
      setTheme("light");
    } else if (theme === "dark") {
      html.classList.add("mdui-theme-dark");
      setTheme("dark");
    } else {
      // 自动模式
      const monetEnabled = savedMonet !== "false";
      if (monetEnabled) {
        // 莫奈取色开启：使用 mdui-theme-auto
        html.classList.add("mdui-theme-auto");
        setTheme("auto");
      } else {
        // 莫奈取色关闭：根据系统偏好设置主题
        html.classList.add(isDark ? "mdui-theme-dark" : "mdui-theme-light");
        setTheme(isDark ? "dark" : "light");
      }
    }
  }

  setupDialogs(): void {
    const importMenu = document.getElementById("import-menu") as any;

    document
      .getElementById("import-node-link")
      ?.addEventListener("click", () => {
        importMenu.open = false;
        (document.getElementById("node-link-dialog") as any).open = true;
      });

    document
      .getElementById("import-full-config")
      ?.addEventListener("click", () => {
        importMenu.open = false;
        this.showConfigDialog();
      });

    document
      .getElementById("node-link-cancel")
      ?.addEventListener("click", () => {
        (document.getElementById("node-link-dialog") as any).open = false;
      });

    document
      .getElementById("node-link-save")
      ?.addEventListener("click", async () => {
        await this.configPage.importNodeLink();
      });

    // 订阅对话框事件
    document
      .getElementById("import-subscription")
      ?.addEventListener("click", () => {
        importMenu.open = false;
        (document.getElementById("subscription-dialog") as any).open = true;
      });

    document
      .getElementById("subscription-cancel")
      ?.addEventListener("click", () => {
        (document.getElementById("subscription-dialog") as any).open = false;
      });

    document
      .getElementById("subscription-save")
      ?.addEventListener("click", async () => {
        await this.configPage.saveSubscription();
      });

    document
      .getElementById("config-cancel-btn")
      ?.addEventListener("click", () => {
        (document.getElementById("config-dialog") as any).open = false;
      });

    document.getElementById("uid-cancel-btn")?.addEventListener("click", () => {
      (document.getElementById("uid-dialog") as any).open = false;
    });

    document
      .getElementById("config-save-btn")
      ?.addEventListener("click", async () => {
        await this.configPage.saveConfig();
      });

    document
      .getElementById("app-selector-cancel")
      ?.addEventListener("click", () => {
        (document.getElementById("app-selector-dialog") as any).open = false;
      });

    document
      .getElementById("app-selector-search")
      ?.addEventListener("input", (e: any) => {
        this.appPage.filterApps(e.target.value);
      });

    const checkUpdateBtn = document.getElementById("check-update-btn") as any;
    if (checkUpdateBtn) {
      checkUpdateBtn.addEventListener("click", () => {
        checkUpdateBtn.disabled = true;
        checkUpdateBtn.loading = true;

        setTimeout(async () => {
          try {
            const result: any = await SettingsService.updateXray();

            if (result.success) {
              toast(result.message);
              if (!result.isLatest) {
                setTimeout(() => this.statusPage.update(), 1500);
              }
            } else {
              toast(
                I18nService.t("common.update_failed") +
                  (result.error || result.message),
              );
            }
          } catch (error: any) {
            toast(I18nService.t("common.check_failed") + error.message);
          } finally {
            checkUpdateBtn.disabled = false;
            checkUpdateBtn.loading = false;
          }
        }, 50);
      });
    }
  }

  setupAppSelector(): void {
    try {
      const addAppBtn = document.getElementById("add-uid-btn");

      if (addAppBtn) {
        addAppBtn.addEventListener("click", () => {
          this.appPage.showAppSelector();
        });
      }
    } catch (error) {
      console.error(">> setupAppSelector: ERROR -", error);
    }
  }

  async confirm(message: string): Promise<boolean> {
    return new Promise((resolve) => {
      const dialog = document.getElementById("confirm-dialog") as any;
      const messageEl = document.getElementById("confirm-message");
      const okBtn = document.getElementById("confirm-ok-btn");
      const cancelBtn = document.getElementById("confirm-cancel-btn");

      if (!dialog || !messageEl || !okBtn || !cancelBtn) {
        resolve(false);
        return;
      }

      messageEl.innerHTML = message.replace(/\n/g, "<br>");

      const newOkBtn = okBtn.cloneNode(true);
      const newCancelBtn = cancelBtn.cloneNode(true);
      okBtn.parentNode?.replaceChild(newOkBtn, okBtn);
      cancelBtn.parentNode?.replaceChild(newCancelBtn, cancelBtn);

      newOkBtn.addEventListener("click", () => {
        dialog.open = false;
        resolve(true);
      });

      newCancelBtn.addEventListener("click", () => {
        dialog.open = false;
        resolve(false);
      });

      dialog.open = true;
    });
  }

  /**
   * 显示骨架屏加载动画
   * @param {HTMLElement} container - 容器元素
   * @param {number} count - 骨架项数量
   * @param {Object} options - 配置选项
   * @param {boolean} options.showIcon - 是否显示圆形图标占位符（默认 true，适用于应用列表）
   */
  showSkeleton(
    container: HTMLElement,
    count = 3,
    options: SkeletonOptions = {},
  ): void {
    const { showIcon = true } = options;
    container.innerHTML = "";
    for (let i = 0; i < count; i++) {
      const item = document.createElement("mdui-list-item");
      if (showIcon) {
        // 带图标的骨架屏（适用于应用列表）
        item.innerHTML = `
                    <div style="display: flex; align-items: center; gap: 12px; width: 100%; padding: 8px 0;">
                        <div class="skeleton skeleton-circle" style="width: 40px; height: 40px;"></div>
                        <div style="flex: 1;">
                            <div class="skeleton skeleton-text" style="width: 60%; height: 16px; margin-bottom: 8px;"></div>
                            <div class="skeleton skeleton-text" style="width: 40%; height: 12px;"></div>
                        </div>
                    </div>
                `;
      } else {
        // 不带图标的骨架屏（适用于配置文件列表）
        item.innerHTML = `
                    <div style="display: flex; flex-direction: column; gap: 8px; width: 100%; padding: 12px 0;">
                        <div class="skeleton skeleton-text" style="width: 50%; height: 16px;"></div>
                        <div class="skeleton skeleton-text" style="width: 70%; height: 12px;"></div>
                    </div>
                `;
      }
      container.appendChild(item);
    }
  }

  updateAllPages(): void {
    try {
      this.statusPage.update();
    } catch (error) {
      console.error("Error in updateAllPages:", error);
    }
  }

  async showConfigDialog(filename: string | null = null): Promise<void> {
    await this.configPage.showDialog(filename);
  }
}
