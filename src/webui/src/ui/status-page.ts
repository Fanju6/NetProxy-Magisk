import { StatusService } from "../services/status-service.js";
import { I18nService } from "../i18n/i18n-service.js";
import { toast } from "../utils/toast.js";
import Sortable from "sortablejs";
import uPlot from "uplot";
import "uplot/dist/uPlot.min.css";
import type { UI } from "./ui-core.js";

interface SpeedHistory {
  time: number[];
  download: number[];
  upload: number[];
}

interface TrafficStats {
  rx: number;
  tx: number;
}

/**
 * 将国家代码转换为国旗 Emoji
 * @param countryCode - 两位国家代码（如 CN、US）
 * @returns 国旗 Emoji 字符串
 */
const countryCodeToEmoji = (countryCode: string): string => {
  const code = countryCode.toUpperCase();
  if (code.length !== 2) {
    return countryCode;
  }
  const firstLetter = code.codePointAt(0)! - 0x41 + 0x1f1e6;
  const secondLetter = code.codePointAt(1)! - 0x41 + 0x1f1e6;
  return String.fromCodePoint(firstLetter) + String.fromCodePoint(secondLetter);
};

/**
 * 为十六进制颜色添加透明度
 * @param hexColor - 十六进制颜色值（支持 #RRGGBB 或 #RRGGBBAA 格式）
 * @param alpha - 两位十六进制透明度值
 * @returns 带透明度的颜色值
 */
const addColorAlpha = (hexColor: string, alpha: string): string => {
  const hex = hexColor.replace("#", "");
  if (hex.length === 8) {
    const r = parseInt(hex.substring(0, 2), 16);
    const g = parseInt(hex.substring(2, 4), 16);
    const b = parseInt(hex.substring(4, 6), 16);
    const a = parseInt(alpha, 16) / 255;
    return `rgba(${r}, ${g}, ${b}, ${a.toFixed(2)})`;
  }
  return "#" + hex + alpha;
};

/**
 * 状态页面管理器
 */
export class StatusPageManager {
  ui: UI;
  uptimeStartTime: number | null;
  uptimeInterval: ReturnType<typeof setInterval> | null;
  speedChart: any;
  speedHistory: SpeedHistory;
  maxDataPoints: number;
  trafficStats: TrafficStats;
  sortable: Sortable | null;
  isEditing: boolean;

  constructor(ui: UI) {
    this.ui = ui;
    this.uptimeStartTime = null;
    this.uptimeInterval = null;

    // 网速图表相关
    this.speedChart = null;
    this.speedHistory = {
      time: [],
      download: [],
      upload: [],
    };
    this.maxDataPoints = 10;

    // 流量统计
    this.trafficStats = { rx: 0, tx: 0 };

    // 拖拽编辑相关
    this.sortable = null;
    this.isEditing = false;

    // 恢复保存的布局
    this.loadLayout();

    // 绑定事件
    this.bindEvents();
  }

  toggleEditMode(): void {
    this.isEditing = !this.isEditing;
    const grid = document.getElementById("dashboard-grid");
    const btn = document.getElementById("edit-dashboard-btn");
    const addPanel = document.getElementById("add-widget-panel");

    if (!grid) return;

    if (this.isEditing) {
      grid.classList.add("editing-mode");
      if (btn) btn.classList.add("active");
      if (addPanel) {
        addPanel.classList.add("visible");
        this.renderAddWidgetList();
      }

      // 启用 Sortable
      this.sortable = new Sortable(grid, {
        animation: 200,
        ghostClass: "sortable-ghost",
        dragClass: "sortable-drag",
        delay: 200,
        delayOnTouchOnly: true,
        filter: ".card-delete-btn", // 过滤器，防止拖拽删除按钮
        onEnd: () => {
          this.saveLayout();
        },
      });
      // toast(I18nService.t('common.edit_mode_enabled') || '已进入编辑模式');
    } else {
      grid.classList.remove("editing-mode");
      if (btn) btn.classList.remove("active");
      if (addPanel) addPanel.classList.remove("visible");

      // 销毁 Sortable
      if (this.sortable) {
        this.sortable.destroy();
        this.sortable = null;
      }
      this.saveLayout();
      // toast(I18nService.t('common.layout_saved') || '布局已保存');
    }
  }

  saveLayout(): void {
    const grid = document.getElementById("dashboard-grid");
    if (!grid) return;

    // 获取所有显示中的卡片 ID 顺序
    const order = Array.from(grid.children)
      .filter((el) => (el as HTMLElement).style.display !== "none")
      .map((el) => el.id)
      .filter((id) => id);

    localStorage.setItem("dashboard-layout", JSON.stringify(order));
  }

  loadLayout(): void {
    const savedLayout = localStorage.getItem("dashboard-layout");
    const grid = document.getElementById("dashboard-grid");
    if (!grid) return;

    const cards = Array.from(grid.children) as HTMLElement[];

    if (savedLayout) {
      try {
        const order: string[] = JSON.parse(savedLayout);
        const fragment = document.createDocumentFragment();

        // 1. 按顺序添加保存的卡片 (显示)
        order.forEach((id) => {
          const card = cards.find((el) => el.id === id);
          if (card) {
            card.style.display = ""; // 确保显示
            fragment.appendChild(card);
          }
        });

        // 2. 处理未在保存列表中的卡片 (隐藏)
        cards.forEach((card) => {
          if (!order.includes(card.id)) {
            card.style.display = "none";
            fragment.appendChild(card); // 依然在 DOM 中，只是隐藏
          }
        });

        grid.appendChild(fragment);
      } catch (e) {
        console.error("Failed to load dashboard layout", e);
      }
    }
  }

  bindEvents(): void {
    const grid = document.getElementById("dashboard-grid");
    if (grid) {
      // 代理删除按钮点击
      grid.addEventListener("click", (e: Event) => {
        const target = e.target as HTMLElement;
        const deleteBtn = target.closest(".card-delete-btn");
        if (deleteBtn && this.isEditing) {
          const card = deleteBtn.closest(".common-card");
          if (card && card.id) {
            this.removeCard(card.id);
            e.stopPropagation(); // 防止触发卡片点击
          }
        }
      });
    }

    const closeAddBtn = document.getElementById("close-add-widget-btn");
    if (closeAddBtn) {
      closeAddBtn.addEventListener("click", () => {
        this.toggleEditMode();
      });
    }
  }

  removeCard(id: string): void {
    const card = document.getElementById(id);
    if (card) {
      card.style.display = "none";
      this.saveLayout();
      this.renderAddWidgetList(); // 刷新添加列表
    }
  }

  addCard(id: string): void {
    const card = document.getElementById(id);
    const grid = document.getElementById("dashboard-grid");
    if (card && grid) {
      card.style.display = ""; // 显示
      grid.appendChild(card); // 移动到末尾
      this.saveLayout();
      this.renderAddWidgetList(); // 刷新添加列表
    }
  }

  renderAddWidgetList(): void {
    const container = document.getElementById("add-widget-list");
    const grid = document.getElementById("dashboard-grid");
    if (!container || !grid) return;

    container.innerHTML = "";
    const hiddenCards = Array.from(grid.children).filter(
      (el) => (el as HTMLElement).style.display === "none",
    );

    if (hiddenCards.length === 0) {
      container.innerHTML = `<div style="color: var(--monet-on-surface-variant); font-size: 14px; padding: 8px;">无可用组件</div>`;
      return;
    }

    hiddenCards.forEach((card) => {
      const titleEl = card.querySelector(".card-title");
      const iconEl = card.querySelector(".card-icon");
      const title = titleEl ? titleEl.textContent : card.id;
      const iconName = iconEl ? iconEl.getAttribute("name") : "widgets";

      const item = document.createElement("div");
      item.className = "widget-preview-item";
      item.innerHTML = `
                <mdui-icon name="${iconName}"></mdui-icon>
                <span>${title}</span>
                <mdui-icon name="add" style="font-size: 16px; margin-left: 4px;"></mdui-icon>
            `;
      item.onclick = () => {
        this.addCard(card.id);
      };
      container.appendChild(item);
    });
  }

  async update(): Promise<void> {
    try {
      const { status } = await StatusService.getStatus();

      // 更新 FAB 按钮状态
      const fab = document.getElementById("service-fab") as any;
      const fabContainer = document.getElementById("dashboard-fab");

      if (status === "running") {
        if (fab) fab.icon = "stop";
        if (fabContainer) fabContainer.classList.add("running");

        if (!this.uptimeInterval) {
          const uptime = await StatusService.getUptime();
          if (
            uptime &&
            uptime !== "--" &&
            uptime !== "N/A" &&
            !uptime.includes("failed")
          ) {
            this.startUptimeTimer(uptime);
          }
        }
      } else {
        if (fab) fab.icon = "play_arrow";
        if (fabContainer) fabContainer.classList.remove("running");
        const fabRuntime = document.getElementById("fab-runtime");
        if (fabRuntime) fabRuntime.textContent = "";
        this.stopUptimeTimer();
      }

      // 先初始化图表 (确保有初始数据)
      if (!this.speedChart) {
        this.initSpeedChart();
      }

      // 然后更新网速
      await this.updateNetworkSpeed();

      // 更新 IP 和流量信息
      await this.updateIPAndTraffic();

      // 更新出站模式 UI
      await this.updateModeUI();

      // 更新系统状态 (CPU/Mem)
      const sysStatus = await StatusService.getSystemStatus();
      const cpuEl = document.getElementById("cpu-usage");
      if (cpuEl) cpuEl.textContent = `${sysStatus.cpu}%`;

      const memEl = document.getElementById("memory-usage");
      if (memEl) memEl.textContent = `${sysStatus.mem.percentage}%`;
    } catch (error) {
      console.error("Update status failed:", error);
    }
  }

  async updateNetworkSpeed(): Promise<void> {
    try {
      const speed = await StatusService.getNetworkSpeed();
      const downloadValue =
        parseFloat(speed.download.replace(" KB/s", "").trim()) || 0;
      const uploadValue =
        parseFloat(speed.upload.replace(" KB/s", "").trim()) || 0;

      // 更新 Header 速度显示
      const headerSpeedEl = document.getElementById("header-speed-text");
      if (headerSpeedEl) {
        const formatSpeed = (val: number) => {
          if (val >= 1024) return `${(val / 1024).toFixed(1)} MB/s`;
          if (val >= 1) return `${val.toFixed(0)} KB/s`;
          return `${(val * 1024).toFixed(0)} B/s`;
        };
        headerSpeedEl.textContent = `↑ ${formatSpeed(uploadValue)}   ↓ ${formatSpeed(downloadValue)}`;
      }

      // 获取下一个索引值 (基于最后一个 X 值 + 1)
      let nextIndex = 0;
      if (this.speedHistory.time.length > 0) {
        nextIndex =
          this.speedHistory.time[this.speedHistory.time.length - 1] + 1;
      }

      this.speedHistory.time.push(nextIndex);
      this.speedHistory.download.push(downloadValue);
      this.speedHistory.upload.push(uploadValue);

      // 限制窗口大小
      if (this.speedHistory.time.length > this.maxDataPoints) {
        this.speedHistory.time.shift();
        this.speedHistory.download.shift();
        this.speedHistory.upload.shift();
      }

      // 更新图表
      if (this.speedChart) {
        this.speedChart.setData([
          this.speedHistory.time,
          this.speedHistory.download,
          this.speedHistory.upload,
        ]);
      }
    } catch (error) {
      console.error("Update network speed failed:", error);
    }
  }

  async updateIPAndTraffic(): Promise<void> {
    // 内网 IP
    try {
      const ips = await StatusService.getInternalIP();
      const internalEl = document.getElementById("internal-ip");
      if (internalEl) {
        if (ips && ips.length > 0) {
          internalEl.textContent = ips[0].ip;
          internalEl.title = ips.map((i) => `${i.ip} (${i.iface})`).join("\n");
        } else {
          internalEl.textContent =
            I18nService.t("status.no_network") || "无网络";
        }
      }
    } catch (e) {
      const el = document.getElementById("internal-ip");
      if (el) el.textContent = "--";
    }

    // 外网 IP
    this.updateExternalIP();

    // 流量统计
    try {
      const stats = await StatusService.getTrafficStats();
      this.trafficStats = stats;

      // 更新上传/下载显示
      const uploadEl = document.getElementById("traffic-upload");
      const downloadEl = document.getElementById("traffic-download");

      if (uploadEl && downloadEl) {
        const { value: uploadValue, unit: uploadUnit } = this.formatTraffic(
          stats.tx,
        );
        const { value: downloadValue, unit: downloadUnit } = this.formatTraffic(
          stats.rx,
        );

        uploadEl.textContent = uploadValue;
        if (uploadEl.nextElementSibling)
          uploadEl.nextElementSibling.textContent = uploadUnit;
        downloadEl.textContent = downloadValue;
        if (downloadEl.nextElementSibling)
          downloadEl.nextElementSibling.textContent = downloadUnit;
      }

      // 更新环形图
      this.updateDonutChart(stats.tx, stats.rx);
    } catch (e) {
      const uploadEl = document.getElementById("traffic-upload");
      const downloadEl = document.getElementById("traffic-download");
      if (uploadEl) uploadEl.textContent = "0";
      if (downloadEl) downloadEl.textContent = "0";
    }
  }

  formatTraffic(bytes: number): { value: string; unit: string } {
    if (bytes >= 1024 * 1024 * 1024) {
      return { value: (bytes / 1024 / 1024 / 1024).toFixed(2), unit: "GB" };
    } else if (bytes >= 1024 * 1024) {
      return { value: (bytes / 1024 / 1024).toFixed(1), unit: "MB" };
    } else if (bytes >= 1024) {
      return { value: (bytes / 1024).toFixed(0), unit: "KB" };
    }
    return { value: bytes.toString(), unit: "B" };
  }

  /**
   * 更新外网 IP 和国旗图标
   * 使用 fire-and-forget 模式，不影响其他 UI 更新
   */
  updateExternalIP(): void {
    const ipEl = document.getElementById("external-ip");
    const iconEl = document.getElementById("network-detection-icon");

    // 显示加载状态
    if (ipEl) ipEl.innerHTML = '<span class="loading-spinner"></span>';

    StatusService.getExternalIPInfo()
      .then((ipInfo) => {
        // 更新图标位置显示国旗
        if (iconEl) {
          if (ipInfo) {
            const flag = countryCodeToEmoji(ipInfo.countryCode);
            if (
              iconEl instanceof HTMLElement &&
              iconEl.tagName.toLowerCase() === "span"
            ) {
              iconEl.className = "flag-icon twemoji";
              iconEl.textContent = flag;
            } else {
              iconEl.outerHTML = `<span id="network-detection-icon" class="flag-icon twemoji">${flag}</span>`;
            }
          } else {
            iconEl.outerHTML = `<mdui-icon name="network_check" class="card-icon" id="network-detection-icon"></mdui-icon>`;
          }
        }

        // 更新 IP 显示
        if (ipEl) {
          ipEl.textContent = ipInfo?.ip || "--";
        }
      })
      .catch(() => {
        // 恢复默认图标
        if (iconEl) {
          iconEl.outerHTML = `<mdui-icon name="network_check" class="card-icon" id="network-detection-icon"></mdui-icon>`;
        }
        if (ipEl) ipEl.textContent = "--";
      });
  }

  updateDonutChart(upload: number, download: number): void {
    const container = document.getElementById("traffic-donut");
    if (!container) return;

    const total = upload + download;
    if (total === 0) {
      container.innerHTML = `
                <svg viewBox="0 0 100 100" class="active">
                    <circle cx="50" cy="50" r="35" fill="none" stroke="var(--mdui-color-outline-variant, #ccc)" stroke-width="10"/>
                </svg>
            `;
      return;
    }

    const uploadPercent = upload / total;
    const radius = 35;
    const circumference = 2 * Math.PI * radius;
    const uploadDash = uploadPercent * circumference;
    const downloadDash = (1 - uploadPercent) * circumference;

    container.innerHTML = `
            <svg viewBox="0 0 100 100" class="active">
                <circle cx="50" cy="50" r="${radius}" fill="none" 
                    stroke="var(--monet-secondary, #2196F3)" stroke-width="10"
                    stroke-dasharray="${downloadDash} ${circumference}"
                    transform="rotate(-90 50 50)"/>
                <circle cx="50" cy="50" r="${radius}" fill="none" 
                    stroke="var(--monet-primary, #4CAF50)" stroke-width="10"
                    stroke-dasharray="${uploadDash} ${circumference}"
                    stroke-dashoffset="${-downloadDash}"
                    transform="rotate(-90 50 50)"/>
            </svg>
        `;
  }

  initSpeedChart(): void {
    const container = document.getElementById("speed-chart-container");
    if (!container || container.clientWidth === 0) {
      // 容器还没准备好，稍后再试
      setTimeout(() => this.initSpeedChart(), 100);
      return;
    }

    const isDark =
      document.documentElement.classList.contains("mdui-theme-dark") ||
      (document.documentElement.classList.contains("mdui-theme-auto") &&
        window.matchMedia("(prefers-color-scheme: dark)").matches);

    const primaryColor =
      getComputedStyle(document.documentElement)
        .getPropertyValue("--monet-primary")
        .trim() || "#4CAF50";
    const secondaryColor =
      getComputedStyle(document.documentElement)
        .getPropertyValue("--monet-secondary")
        .trim() || "#2196F3";

    // 初始化数据: 使用两个点 (0, 1) 但值为 0，形成初始横线铺满宽度
    this.speedHistory.time = [0, 1];
    this.speedHistory.download = [0, 0];
    this.speedHistory.upload = [0, 0];

    const opts: any = {
      width: container.clientWidth,
      height: container.clientHeight || 80,
      series: [
        {},
        {
          label: "Download",
          stroke: secondaryColor,
          width: 2,
          paths: uPlot.paths.spline(),
          points: { show: false }, // 隐藏数据点
          // 填充满底部: 使用极小值确保填充覆盖到图表底部（负值区域）
          fillTo: -1e9,
          fill: (u: any, seriesIdx: number) => {
            const gradient = u.ctx.createLinearGradient(0, 0, 0, u.height);
            gradient.addColorStop(0, addColorAlpha(secondaryColor, "60"));
            gradient.addColorStop(1, addColorAlpha(secondaryColor, "1A"));
            return gradient;
          },
        },
        {
          label: "Upload",
          stroke: primaryColor,
          width: 2,
          paths: uPlot.paths.spline(),
          points: { show: false }, // 隐藏数据点
          fillTo: -1e9,
          fill: (u: any, seriesIdx: number) => {
            const gradient = u.ctx.createLinearGradient(0, 0, 0, u.height);
            gradient.addColorStop(0, addColorAlpha(primaryColor, "60"));
            gradient.addColorStop(1, addColorAlpha(primaryColor, "1A"));
            return gradient;
          },
        },
      ],
      axes: [{ show: false }, { show: false }],
      scales: {
        x: {
          time: false,
        },
        y: {
          auto: true,
          // 悬浮效果：将 0 线抬高
          range: (u: any, min: number, max: number) => {
            const effectiveMax = Math.max(max, 100);
            // 底部负值区域作为"悬浮"支撑
            return [-effectiveMax * 0.45, effectiveMax];
          },
        },
      },
      legend: { show: false },
      cursor: { show: false },
      padding: [0, 0, 0, 0],
    };

    this.speedChart = new (uPlot as any)(
      opts,
      [
        this.speedHistory.time,
        this.speedHistory.download,
        this.speedHistory.upload,
      ],
      container,
    );

    // 监听窗口大小变化
    window.addEventListener("resize", () => {
      if (this.speedChart && container && container.clientWidth > 0) {
        this.speedChart.setSize({
          width: container.clientWidth,
          height: container.clientHeight || 80,
        });
      }
    });
  }

  startUptimeTimer(uptimeString: string): void {
    const parts = uptimeString.split(/[-:]/);

    let totalSeconds = 0;
    if (parts.length === 4) {
      totalSeconds =
        parseInt(parts[0]) * 86400 +
        parseInt(parts[1]) * 3600 +
        parseInt(parts[2]) * 60 +
        parseInt(parts[3]);
    } else if (parts.length === 3) {
      totalSeconds =
        parseInt(parts[0]) * 3600 +
        parseInt(parts[1]) * 60 +
        parseInt(parts[2]);
    } else if (parts.length === 2) {
      totalSeconds = parseInt(parts[0]) * 60 + parseInt(parts[1]);
    }

    this.uptimeStartTime = Date.now() - totalSeconds * 1000;

    if (this.uptimeInterval) {
      clearInterval(this.uptimeInterval);
    }

    this.updateUptimeDisplay();
    this.uptimeInterval = setInterval(() => this.updateUptimeDisplay(), 1000);
  }

  stopUptimeTimer(): void {
    if (this.uptimeInterval) {
      clearInterval(this.uptimeInterval);
      this.uptimeInterval = null;
    }
    this.uptimeStartTime = null;
  }

  updateUptimeDisplay(): void {
    if (!this.uptimeStartTime) return;

    const elapsed = Math.floor((Date.now() - this.uptimeStartTime) / 1000);
    const hours = Math.floor(elapsed / 3600);
    const minutes = Math.floor((elapsed % 3600) / 60);
    const seconds = elapsed % 60;

    const uptimeStr = `${String(hours).padStart(2, "0")}:${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}`;

    const fabRuntime = document.getElementById("fab-runtime");
    if (fabRuntime) {
      // 添加两个空格作为物理占位符，防止右侧紧贴
      fabRuntime.textContent = uptimeStr + "\u00A0\u00A0";
    }
  }

  // 更新出站模式 UI
  async updateModeUI(): Promise<void> {
    try {
      const currentMode = await StatusService.getOutboundMode();

      // 更新按钮状态
      const modeOptions = document.querySelectorAll(".mode-option");
      modeOptions.forEach((option) => {
        const mode = (option as HTMLElement).dataset.mode;
        if (mode === currentMode) {
          option.classList.add("active");
        } else {
          option.classList.remove("active");
        }
      });

      // 直连模式下隐藏节点页面入口
      const navConfig = document.getElementById("nav-config");
      if (navConfig) {
        navConfig.style.display = currentMode === "direct" ? "none" : "";
      }
    } catch (error) {
      console.error("更新模式 UI 失败:", error);
    }
  }

  // 设置模式按钮点击事件
  setupModeButtons(): void {
    const modeOptions = document.querySelectorAll(".mode-option");

    modeOptions.forEach((option) => {
      option.addEventListener("click", async () => {
        const mode = (option as HTMLElement).dataset.mode!; // Non-null assertion for data set in HTML

        // 避免重复点击
        if (
          option.classList.contains("active") ||
          option.classList.contains("loading")
        ) {
          return;
        }

        // 显示加载状态
        option.classList.add("loading");

        // 立即更新 UI (乐观更新)
        const navConfig = document.getElementById("nav-config");
        if (navConfig) {
          navConfig.style.display = mode === "direct" ? "none" : "";
        }

        // 记录当前激活的按钮用于回滚
        const previousActive = document.querySelector(
          ".mode-option.active",
        ) as HTMLElement | null;

        // 立即更新按钮状态
        modeOptions.forEach((opt) => opt.classList.remove("active"));
        option.classList.add("active");

        try {
          const success = await StatusService.setOutboundMode(mode);

          if (!success) {
            // 切换失败，恢复状态
            modeOptions.forEach((opt) => opt.classList.remove("active"));
            if (previousActive) previousActive.classList.add("active");
            if (navConfig) {
              navConfig.style.display =
                previousActive?.dataset.mode === "direct" ? "none" : "";
            }
            toast(I18nService.t("status.mode_switch_failed") || "模式切换失败");
          }
        } catch (error: any) {
          console.error("模式切换失败:", error);
          toast(error.message || "模式切换失败");
        } finally {
          option.classList.remove("loading");
        }
      });
    });
  }

  /**
   * 更新网速图表颜色
   * 当主题色改变时调用此方法刷新图表颜色
   */
  updateSpeedChartColors(): void {
    if (!this.speedChart) return;

    this.speedChart.destroy();
    this.speedChart = null;
    this.initSpeedChart();
  }
}
