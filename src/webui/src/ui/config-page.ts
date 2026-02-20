import { ConfigService } from "../services/config-service.js";
import { StatusService } from "../services/status-service.js";
import { toast } from "../utils/toast.js";
import { I18nService } from "../i18n/i18n-service.js";
import type { UI } from "./ui-core.js";

interface ConfigGroup {
  name: string;
  dirName?: string;
  type?: string;
  configs: string[];
}

interface ConfigInfo {
  protocol: string;
  address: string;
  port: string;
}

interface LatencyCache {
  latency: number;
  latencyStr: string;
}

/**
 * 配置页面管理器 - 支持分组显示
 */
export class ConfigPageManager {
  ui: UI;
  currentOpenDropdown: any;
  _tabEventBound: boolean;
  _cachedGroups: ConfigGroup[] | null;
  _cachedCurrentConfig: string | null;
  _cachedConfigInfos: Map<string, Map<string, ConfigInfo>>;
  _latencyCache: Map<string, LatencyCache>;
  _loadingChunks: Set<string>;
  _selectedTab: string | null;
  observer: IntersectionObserver;
  _tabChangeHandler: ((e: Event) => Promise<void>) | null;

  constructor(ui: UI) {
    this.ui = ui;
    this.currentOpenDropdown = null;
    this._tabEventBound = false; // 防止重复绑定 tab 事件
    this._cachedGroups = null;
    this._cachedCurrentConfig = null;
    this._cachedConfigInfos = new Map(); // groupName -> Map<filename>, info
    this._latencyCache = new Map();
    this._loadingChunks = new Set(); // 防止并发加载同一 chunk
    this._selectedTab = null; // 持久化当前选中的 tab
    this._tabChangeHandler = null;

    // 懒加载观察器
    this.observer = new IntersectionObserver(
      (entries) => {
        entries.forEach((entry) => {
          if (entry.isIntersecting) {
            const item = entry.target as HTMLElement;
            const groupName = item.dataset.groupName;
            const filename = item.dataset.filename;

            if (groupName && filename) {
              this.loadConfigForItem(item, groupName, filename);
            }
            this.observer.unobserve(item);
          }
        });
      },
      { rootMargin: "200px" },
    );
  }

  /**
   * 从配置内容解析出站信息
   */
  parseOutboundInfo(content: string): ConfigInfo {
    try {
      const config = JSON.parse(content);
      const outbounds = config.outbounds || [];

      for (const outbound of outbounds) {
        const protocol = outbound.protocol;
        if (!protocol || ["freedom", "blackhole", "dns"].includes(protocol)) {
          continue;
        }

        let address = "";
        let port = "";

        if (outbound.settings) {
          if (outbound.settings.vnext && outbound.settings.vnext[0]) {
            address = outbound.settings.vnext[0].address || "";
            port = outbound.settings.vnext[0].port || "";
          } else if (
            outbound.settings.servers &&
            outbound.settings.servers[0]
          ) {
            address = outbound.settings.servers[0].address || "";
            port = outbound.settings.servers[0].port || "";
          }
        }

        return { protocol, address, port };
      }

      return {
        protocol: "direct",
        address: I18nService.t("config.parser.direct"),
        port: "",
      };
    } catch (e) {
      return { protocol: "unknown", address: "", port: "" };
    }
  }

  // 刷新数据并渲染（首次加载或手动刷新时调用）
  async update(forceRefresh = false) {
    try {
      // 如果已有缓存且不是强制刷新，直接渲染（页面切换回来时使用缓存）
      if (
        !forceRefresh &&
        this._cachedGroups &&
        this._cachedGroups.length > 0
      ) {
        await this.render();
        return;
      }

      // 1. 获取目录结构（快速，无详情）
      const groups = await ConfigService.getConfigGroups();

      // 2. 更新缓存
      this._cachedGroups = groups;
      const { config } = await StatusService.getStatus();
      this._cachedCurrentConfig = config;

      // 3. 立即渲染结构
      await this.render();

      // 4. 不再主动加载所有详情，依靠 IntersectionObserver 懒加载
    } catch (error) {}
  }

  // 加载单个节点详情（实际会加载周围的一批）
  async loadConfigForItem(item, groupName, filename) {
    const group = this._cachedGroups.find((g) => g.name === groupName);
    if (!group) return;

    // 如果该分组尚未初始化 Map
    if (!this._cachedConfigInfos.has(groupName)) {
      this._cachedConfigInfos.set(groupName, new Map());
    }
    const groupInfos = this._cachedConfigInfos.get(groupName);
    if (!groupInfos) return;

    // 如果已经有数据，直接渲染（可能刚加载完）
    const existingInfo = groupInfos.get(filename);
    if (existingInfo && existingInfo.protocol !== "loading...") {
      this.updateItemUI(item, existingInfo);
      return;
    }

    // 找到该文件在列表中的索引
    const index = group.configs.indexOf(filename);
    if (index === -1) return;

    // 加载该索引附近的一批文件
    const CHUNK_SIZE = 20;
    const chunkIndex = Math.floor(index / CHUNK_SIZE);
    const chunkKey = `${groupName}:${chunkIndex}`;

    // 检查是否正在加载该 chunk
    if (this._loadingChunks.has(chunkKey)) return;
    this._loadingChunks.add(chunkKey);

    const start = chunkIndex * CHUNK_SIZE;
    const end = Math.min((chunkIndex + 1) * CHUNK_SIZE, group.configs.length);

    const filesToLoad = group.configs.slice(start, end);
    // 过滤掉已经加载过的
    const pendingFiles = filesToLoad.filter((f) => {
      const info = groupInfos.get(f);
      return !info || info.protocol === "loading..."; // 重新加载 loading 状态的
    });

    if (pendingFiles.length === 0) return;

    // 标记为正在加载，避免重复请求
    pendingFiles.forEach((f) =>
      groupInfos.set(f, { protocol: "loading...", address: "", port: "" }),
    );

    // 构建完整路径
    const filePaths = pendingFiles.map((f) =>
      group.dirName ? `${group.dirName}/${f}` : f,
    );

    // 批量读取
    const newInfos = await ConfigService.batchReadConfigInfos(filePaths);

    // 更新缓存并刷新 UI
    for (const [fname, info] of newInfos) {
      groupInfos.set(fname, info);
      const targetItem = document.querySelector(
        `.config-item[data-group-name="${groupName}"][data-filename="${fname}"]`,
      );
      if (targetItem) {
        this.updateItemUI(targetItem, info);
      }
    }

    // 移除加载锁
    this._loadingChunks.delete(chunkKey);
  }

  updateItemUI(item, info) {
    // 更新协议
    const protocolLine = item.querySelector(".protocol-line");
    if (protocolLine)
      protocolLine.textContent = (
        info.protocol || I18nService.t("config.parser.unknown")
      ).toUpperCase();

    // 更新地址
    const addressSpan = item.querySelector(".address-span");
    if (addressSpan)
      addressSpan.textContent = info.port
        ? `${info.address}:${info.port}`
        : info.address;
  }

  setLatencyDisplay(latencyLabel, latencyCache) {
    if (!latencyLabel || !latencyCache) return;

    latencyLabel.textContent = latencyCache.latencyStr;
    const ms = parseInt(latencyCache.latencyStr);
    if (!isNaN(ms)) {
      if (ms < 100) {
        latencyLabel.style.color = "#4caf50";
      } else if (ms < 300) {
        latencyLabel.style.color = "#ff9800";
      } else {
        latencyLabel.style.color = "#f44336";
      }
    } else if (
      latencyCache.latencyStr === "failed" ||
      latencyCache.latencyStr === "timeout"
    ) {
      latencyLabel.style.color = "#f44336";
    }
  }

  // 加载分组详情并刷新
  async loadConfigChunk(groupName, startIndex = 0, chunkSize = 20) {
    const group = this._cachedGroups.find((g) => g.name === groupName);
    if (!group || group.configs.length === 0) return;

    if (!this._cachedConfigInfos.has(groupName)) {
      this._cachedConfigInfos.set(groupName, new Map());
    }
    const groupInfos = this._cachedConfigInfos.get(groupName);

    const end = Math.min(startIndex + chunkSize, group.configs.length);
    const filesToLoad = group.configs.slice(startIndex, end);

    const pendingFiles = filesToLoad.filter((f) => {
      const info = groupInfos.get(f);
      return !info || info.protocol === "loading...";
    });

    if (pendingFiles.length === 0) return;

    pendingFiles.forEach((f) =>
      groupInfos.set(f, { protocol: "loading...", address: "", port: "" }),
    );

    const filePaths = pendingFiles.map((f) =>
      group.dirName ? `${group.dirName}/${f}` : f,
    );
    const newInfos = await ConfigService.batchReadConfigInfos(filePaths);

    for (const [fname, info] of newInfos) {
      groupInfos.set(fname, info);
      const targetItem = document.querySelector(
        `.config-item[data-group-name="${groupName}"][data-filename="${fname}"]`,
      );
      if (targetItem) {
        this.updateItemUI(targetItem, info);
      }
    }
  }

  // 仅渲染 UI（使用 mdui-tabs 横向分组）
  async render() {
    const tabsEl = document.getElementById("config-tabs");
    if (!tabsEl) return;

    if (!this._cachedGroups || this._cachedGroups.length === 0) {
      tabsEl.innerHTML =
        '<mdui-tab value="empty">暂无节点</mdui-tab><mdui-tab-panel slot="panel" value="empty"><p style="padding: 16px; text-align: center;">暂无节点</p></mdui-tab-panel>';
      return;
    }

    // 保存当前选中的 tab
    const currentTab =
      this._selectedTab || this._cachedGroups[0]?.name || "default";

    // 检查现有 tabs 是否匹配缓存（如果匹配则跳过重建，只刷新内容）
    const existingTabs = tabsEl.querySelectorAll("mdui-tab");
    const existingNames = Array.from(existingTabs).map((t) => (t as any).value);
    const cachedNames = this._cachedGroups.map((g) => g.name);
    const tabsMatch =
      existingNames.length === cachedNames.length &&
      existingNames.every((name, i) => name === cachedNames[i]);

    if (
      tabsMatch &&
      existingNames.length > 0 &&
      existingNames[0] !== "loading"
    ) {
      // Tabs 结构匹配，只刷新当前 tab 的内容
      const validTab = this._cachedGroups.find((g) => g.name === currentTab)
        ? currentTab
        : this._cachedGroups[0]?.name;
      this._selectedTab = validTab;
      await this.renderActiveTab(validTab);
      return;
    }

    // 清空并重建 tabs（首次加载或结构变化时）
    tabsEl.innerHTML = "";

    // 1. 创建所有 tab 标签
    for (const group of this._cachedGroups) {
      const tab = document.createElement("mdui-tab") as any;
      tab.value = group.name;
      tab.textContent = `${group.name} (${group.configs.length})`;
      tabsEl.appendChild(tab);
    }

    // 1.5 注入滚动样式到 Shadow DOM（等待渲染后）
    requestAnimationFrame(() => {
      const shadowRoot = tabsEl.shadowRoot;
      if (shadowRoot) {
        const container = shadowRoot.querySelector('[part="container"]');
        if (container) {
          (container as HTMLElement).style.cssText =
            "display: flex; flex-wrap: nowrap; overflow-x: auto; -webkit-overflow-scrolling: touch;";
        }

        // 让每个 tab 保持自然宽度不收缩
        const tabs = shadowRoot.querySelectorAll("slot");
        tabs.forEach((slot) => {
          const assignedElements = slot.assignedElements();
          assignedElements.forEach((el) => {
            if (el.tagName === "MDUI-TAB") {
              (el as HTMLElement).style.cssText =
                "flex-shrink: 0; white-space: nowrap;";
            }
          });
        });
      }

      // 同时给 Light DOM 中的 tab 设置样式
      const lightTabs = tabsEl.querySelectorAll("mdui-tab");
      lightTabs.forEach((tab) => {
        tab.style.cssText = "flex-shrink: 0; white-space: nowrap;";
      });
    });

    // 2. 创建所有 tab-panel
    // 2. 创建所有 tab-panel
    for (const group of this._cachedGroups) {
      const panel = document.createElement("mdui-tab-panel") as any;
      panel.slot = "panel";
      panel.value = group.name;

      // 操作栏 (Toolbar)
      const actions = document.createElement("div");
      actions.style.cssText =
        "display: flex; justify-content: space-between; align-items: center; padding: 12px 16px 4px 16px;";

      // 左侧标题
      const title = document.createElement("span");
      title.textContent = I18nService.t("config.node_list");
      title.setAttribute("data-i18n", "config.node_list");
      title.style.cssText =
        "font-size: 14px; font-weight: 500; color: var(--mdui-color-on-surface-variant);";
      actions.appendChild(title);

      // 右侧菜单按钮
      const dropdown = document.createElement("mdui-dropdown");
      dropdown.setAttribute("placement", "bottom-end");

      const triggerBtn = document.createElement("mdui-button-icon");
      triggerBtn.slot = "trigger";
      triggerBtn.setAttribute("icon", "more_vert");
      dropdown.appendChild(triggerBtn);

      const menu = document.createElement("mdui-menu");

      // 全部测试
      const testItem = document.createElement("mdui-menu-item");
      testItem.innerHTML = `<mdui-icon slot="icon" name="playlist_play"></mdui-icon>${I18nService.t("config.menu.test_all")}`;
      testItem.addEventListener("click", () => {
        dropdown.open = false;
        this.testGroupLatency(group.name);
      });
      menu.appendChild(testItem);

      // 按延迟排序
      const sortItem = document.createElement("mdui-menu-item");
      sortItem.innerHTML = `<mdui-icon slot="icon" name="sort"></mdui-icon>${I18nService.t("config.menu.sort")}`;
      sortItem.addEventListener("click", () => {
        dropdown.open = false;
        this.sortGroupNodes(group.name);
      });
      menu.appendChild(sortItem);

      // 清理无效节点
      const cleanItem = document.createElement("mdui-menu-item");
      cleanItem.innerHTML = `<mdui-icon slot="icon" name="delete_sweep"></mdui-icon>${I18nService.t("config.menu.clean")}`;
      cleanItem.style.color = "var(--mdui-color-error)";
      cleanItem.addEventListener("click", () => {
        dropdown.open = false;
        this.deleteInvalidNodes(group.name);
      });
      menu.appendChild(cleanItem);

      // 订阅专用操作
      if (group.type === "subscription") {
        const updateItem = document.createElement("mdui-menu-item");
        updateItem.innerHTML = `<mdui-icon slot="icon" name="refresh"></mdui-icon>${I18nService.t("config.menu.update_sub")}`;
        updateItem.addEventListener("click", () => {
          dropdown.open = false;
          this.updateSubscription(group.dirName, group.name);
        });
        menu.appendChild(updateItem);

        const deleteItem = document.createElement("mdui-menu-item");
        deleteItem.innerHTML = `<mdui-icon slot="icon" name="delete"></mdui-icon>${I18nService.t("config.menu.delete_sub")}`;
        deleteItem.addEventListener("click", () => {
          dropdown.open = false;
          this.deleteSubscription(group.dirName, group.name);
        });
        menu.appendChild(deleteItem);
      }

      dropdown.appendChild(menu);
      actions.appendChild(dropdown);
      panel.appendChild(actions);

      // 创建列表容器
      const list = document.createElement("mdui-list");
      list.id = `config-list-${group.name}`;
      list.className = "config-group-list";
      panel.appendChild(list);

      tabsEl.appendChild(panel);
    }

    // 3. 恢复选中状态
    const validTab = this._cachedGroups.find((g) => g.name === currentTab)
      ? currentTab
      : this._cachedGroups[0]?.name;
    this._selectedTab = validTab; // 保存到实例变量

    // 延迟激活 tab - 使用点击方式更可靠
    requestAnimationFrame(() => {
      setTimeout(async () => {
        if (validTab) {
          // 找到并点击目标 tab
          const targetTab = tabsEl.querySelector(
            `mdui-tab[value="${validTab}"]`,
          );
          if (targetTab) {
            (targetTab as HTMLElement).click();
          }
          // 为当前 tab 加载并渲染内容
          await this.renderActiveTab(validTab);
        }
      }, 100);
    });

    // 5. 绑定 tab 切换事件（每次重新绑定，先移除旧的）
    if (this._tabChangeHandler) {
      tabsEl.removeEventListener("change", this._tabChangeHandler);
    }
    this._tabChangeHandler = async (e: Event) => {
      const target = e.target as any;
      const newTab = target?.value;
      if (newTab) {
        this._selectedTab = newTab;
        await this.renderActiveTab(newTab);
      }
    };
    tabsEl.addEventListener("change", this._tabChangeHandler);
  }

  // 渲染当前激活的 tab 内容
  async renderActiveTab(groupName) {
    const group = this._cachedGroups.find((g) => g.name === groupName);
    if (!group) return;

    const listEl = document.getElementById(`config-list-${groupName}`);
    if (!listEl) return;

    // 如果没有缓存数据，先加载前 10 个
    if (!this._cachedConfigInfos.has(groupName)) {
      this._cachedConfigInfos.set(groupName, new Map());
      await this.loadConfigChunk(groupName, 0, 10);
    }

    const configInfos = this._cachedConfigInfos.get(groupName) || new Map();

    // 渲染列表
    const fragment = document.createDocumentFragment();
    for (const filename of group.configs) {
      const info = configInfos.get(filename);
      const fullPath = group.dirName
        ? `${group.dirName}/${filename}`
        : filename;
      const isCurrent =
        this._cachedCurrentConfig &&
        this._cachedCurrentConfig.endsWith(filename);

      this.renderConfigItem(
        fragment,
        filename,
        fullPath,
        info,
        isCurrent,
        group,
      );
    }

    listEl.innerHTML = "";
    listEl.appendChild(fragment);
  }

  async loadConfigInfos(group) {
    // 构建完整路径列表
    const filePaths = group.configs.map((f) =>
      group.dirName ? `${group.dirName}/${f}` : f,
    );

    // 批量读取所有配置信息（单次 exec）
    return await ConfigService.batchReadConfigInfos(filePaths);
  }

  renderConfigItem(container, filename, fullPath, info, isCurrent, group) {
    const item = document.createElement("mdui-list-item");
    item.setAttribute("clickable", "");
    item.classList.add("config-item");
    item.dataset.groupName = group.name;
    item.dataset.filename = filename;
    item.dataset.filename = filename;

    const displayName = filename.replace(/\.json$/i, "");
    item.setAttribute("headline", displayName);

    const descContainer = document.createElement("div");
    descContainer.slot = "description";
    descContainer.style.cssText =
      "display: flex; flex-direction: column; gap: 2px; width: 100%;";

    // 1. IP地址+端口 (第二行)
    const addressLine = document.createElement("div");
    addressLine.style.cssText =
      "display: flex; justify-content: space-between; align-items: center;";

    const addressSpan = document.createElement("span");
    addressSpan.className = "address-span";
    addressSpan.style.cssText =
      "color: var(--mdui-color-on-surface-variant); font-size: 13px; flex: 1; min-width: 0; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; margin-right: 8px;";
    addressSpan.textContent = info
      ? info.port
        ? `${info.address}:${info.port}`
        : info.address
      : "";
    addressLine.appendChild(addressSpan);
    descContainer.appendChild(addressLine);

    // 2. 协议 (第三行) - 包含协议名和当前状态
    const protocolLine = document.createElement("div");
    protocolLine.style.cssText =
      "color: var(--mdui-color-primary); font-size: 12px; font-weight: 500; margin-top: 2px; display: flex; align-items: center; gap: 8px;";

    const protocolText = document.createElement("span");
    protocolText.className = "protocol-line";
    protocolText.textContent = info
      ? (info.protocol || I18nService.t("config.parser.unknown")).toUpperCase()
      : "LOADING...";
    protocolLine.appendChild(protocolText);

    if (isCurrent) {
      const currentTag = document.createElement("span");
      currentTag.textContent = I18nService.t("config.status.current");
      currentTag.className = "current-tag-text";
      currentTag.style.cssText =
        "font-size: 10px; padding: 1px 4px; border-radius: 4px; background: var(--mdui-color-primary);";
      protocolLine.appendChild(currentTag);
    }

    descContainer.appendChild(protocolLine);

    // 如果没有 info，加入 Observer
    if (!info) {
      this.observer.observe(item);
    }

    const statusContainer = document.createElement("span");
    statusContainer.style.cssText =
      "display: flex; align-items: center; gap: 6px;";

    const latencyLabel = document.createElement("span");
    latencyLabel.className = "latency-label";
    latencyLabel.style.cssText =
      "font-size: 12px; color: var(--mdui-color-on-surface-variant);";

    const cachedLatency = this._latencyCache.get(filename);
    if (cachedLatency) {
      latencyLabel.textContent = cachedLatency.latencyStr;
      this.setLatencyDisplay(latencyLabel, cachedLatency);
    }

    statusContainer.appendChild(latencyLabel);

    addressLine.appendChild(statusContainer);

    item.appendChild(descContainer);

    // 三点菜单
    const dropdown = document.createElement("mdui-dropdown");
    dropdown.setAttribute("placement", "bottom-end");
    dropdown.slot = "end-icon";

    const menuBtn = document.createElement("mdui-button-icon");
    menuBtn.setAttribute("slot", "trigger");
    menuBtn.setAttribute("icon", "more_vert");
    // 阻止所有事件冒泡到父列表项，防止触发 ripple 和选中效果
    menuBtn.addEventListener("click", (e) => {
      e.stopPropagation();
      // 关闭之前打开的下拉菜单
      if (this.currentOpenDropdown && this.currentOpenDropdown !== dropdown) {
        this.currentOpenDropdown.open = false;
      }
      // 更新当前打开的下拉菜单
      this.currentOpenDropdown = dropdown;
    });
    menuBtn.addEventListener("mousedown", (e) => e.stopPropagation());
    menuBtn.addEventListener("pointerdown", (e) => e.stopPropagation());
    menuBtn.addEventListener("touchstart", (e) => e.stopPropagation());
    dropdown.appendChild(menuBtn);

    // 监听下拉菜单关闭事件
    dropdown.addEventListener("closed", () => {
      if (this.currentOpenDropdown === dropdown) {
        this.currentOpenDropdown = null;
      }
    });

    const menu = document.createElement("mdui-menu");

    // 编辑
    const editItem = document.createElement("mdui-menu-item");
    editItem.innerHTML = `<mdui-icon slot="icon" name="edit"></mdui-icon>${I18nService.t("config.menu.edit")}`;
    editItem.addEventListener("click", async (e) => {
      e.stopPropagation();
      dropdown.open = false;
      await this.ui.showConfigDialog(fullPath);
    });
    menu.appendChild(editItem);

    // 测试
    const testItem = document.createElement("mdui-menu-item");
    testItem.innerHTML = `<mdui-icon slot="icon" name="speed"></mdui-icon>${I18nService.t("config.menu.test")}`;
    testItem.addEventListener("click", async (e) => {
      e.stopPropagation();
      dropdown.open = false;
      await this.testConfig(displayName, info.address, item);
    });
    menu.appendChild(testItem);

    // 删除（非当前配置可删除）
    if (!isCurrent) {
      const deleteItem = document.createElement("mdui-menu-item");
      deleteItem.innerHTML = `<mdui-icon slot="icon" name="delete"></mdui-icon>${I18nService.t("config.menu.delete")}`;
      deleteItem.style.color = "var(--mdui-color-error)";
      deleteItem.addEventListener("click", async (e) => {
        e.stopPropagation();
        dropdown.open = false;
        await this.deleteConfig(fullPath, displayName);
      });
      menu.appendChild(deleteItem);
    }

    dropdown.appendChild(menu);
    item.appendChild(dropdown);

    item.addEventListener("click", () => {
      if (!isCurrent) {
        this.switchConfig(fullPath, displayName);
      }
    });

    container.appendChild(item);
  }

  async testConfig(displayName, address, itemElement) {
    const latencyLabel = itemElement?.querySelector(".latency-label");

    if (!address || address === I18nService.t("config.parser.direct")) {
      if (latencyLabel)
        latencyLabel.textContent = I18nService.t("config.status.direct");
      return;
    }

    try {
      if (latencyLabel) {
        latencyLabel.textContent = I18nService.t("config.status.testing");
        latencyLabel.style.color = "var(--mdui-color-on-surface-variant)";
      }
      const latencyStr = await StatusService.getPingLatency(address);
      let latencyVal = 9999;

      const displayStr =
        latencyStr === "timeout"
          ? I18nService.t("config.status.timeout")
          : latencyStr === "failed"
            ? I18nService.t("config.status.failed")
            : latencyStr;
      const ms = parseInt(latencyStr);
      if (!isNaN(ms)) latencyVal = ms;

      const latencyCache = { latency: latencyVal, latencyStr: latencyStr };
      if (latencyLabel) {
        latencyLabel.textContent = displayStr;
        this.setLatencyDisplay(latencyLabel, latencyCache);
      }

      // 缓存测试结果到独立缓存
      const filename = itemElement?.dataset.filename || displayName + ".json";
      this._latencyCache.set(filename, latencyCache);
    } catch (error) {
      if (latencyLabel) {
        latencyLabel.textContent = I18nService.t("config.status.failed");
        latencyLabel.style.color = "#f44336";
      }
    }
  }

  // 批量测试延迟
  async testGroupLatency(groupName) {
    const group = this._cachedGroups.find((g) => g.name === groupName);
    if (!group) return;

    const listEl = document.getElementById(`config-list-${groupName}`);
    if (!listEl) return;

    const items = Array.from(listEl.querySelectorAll(".config-item"));
    const total = items.length;
    if (total === 0) {
      toast(I18nService.t("config.toast.no_nodes"));
      return;
    }

    const toastId = toast(
      I18nService.t("config.toast.start_test", { count: String(total) }),
      true,
    ); // 持续显示

    // 无限制并发，同时测试所有节点
    await Promise.all(
      items.map(async (item) => {
        const filename = (item as HTMLElement).dataset.filename;
        const info = this._cachedConfigInfos.get(groupName)?.get(filename);
        if (info && info.address) {
          await this.testConfig(
            filename.replace(/\.json$/i, ""),
            info.address,
            item,
          );
        }
      }),
    );

    // 关闭 toast (重新显示一个自动消失的)
    // mdui 没提供直接关闭 toast 的 api，只能发个新的覆盖
    toast(I18nService.t("config.toast.test_complete"));
  }

  // 按延迟排序
  async sortGroupNodes(groupName) {
    const group = this._cachedGroups.find((g) => g.name === groupName);
    if (!group) return;

    const infos = this._cachedConfigInfos.get(groupName);
    if (!infos) return;

    // 排序：有延迟的在前（按数值升序），没延迟的在后（保持原序或排最后）
    group.configs.sort((a, b) => {
      const latA = this._latencyCache.get(a)?.latency ?? 99999;
      const latB = this._latencyCache.get(b)?.latency ?? 99999;
      return latA - latB;
    });

    toast(I18nService.t("config.toast.sorted"));
    await this.renderActiveTab(groupName);
  }

  // 清理无效节点 (失败/超时)
  async deleteInvalidNodes(groupName) {
    const infos = this._cachedConfigInfos.get(groupName);
    if (!infos) {
      toast(I18nService.t("config.toast.need_test"));
      return;
    }

    const invalidFiles = [];
    for (const [filename, info] of infos.entries()) {
      const cachedLatency = this._latencyCache.get(filename);
      if (
        cachedLatency &&
        (cachedLatency.latencyStr === "failed" ||
          cachedLatency.latencyStr === "timeout")
      ) {
        invalidFiles.push(filename);
        this._latencyCache.delete(filename);
      }
    }

    if (invalidFiles.length === 0) {
      toast(I18nService.t("config.toast.no_invalid"));
      return;
    }

    const confirmed = await this.ui.confirm(
      I18nService.t("config.confirm.clean_invalid", {
        count: invalidFiles.length,
      }),
    );
    if (!confirmed) return;

    const group = this._cachedGroups.find((g) => g.name === groupName);
    let successCount = 0;

    for (const filename of invalidFiles) {
      const fullPath = group.dirName
        ? `${group.dirName}/${filename}`
        : filename;
      const result = await ConfigService.deleteConfig(fullPath);
      if (result && result.success) {
        successCount++;
      }
    }

    toast(I18nService.t("config.toast.clean_success", { count: successCount }));
    // 强制刷新
    this._cachedConfigInfos.delete(groupName);
    this.update();
  }

  async deleteConfig(fullPath, displayName) {
    try {
      const confirmed = await this.ui.confirm(
        I18nService.t("config.confirm.delete_node", { name: displayName }),
      );
      if (!confirmed) return;

      const result = await ConfigService.deleteConfig(fullPath);
      if (result && result.success) {
        toast(I18nService.t("config.toast.deleted"));
        // 强制刷新配置列表
        this._cachedGroups = null;
        this._cachedConfigInfos.clear();
        await this.update(true);
      } else {
        toast(
          I18nService.t("config.toast.delete_failed") +
            (result?.error || I18nService.t("common.unknown")),
        );
      }
    } catch (error) {
      toast(I18nService.t("config.toast.delete_failed") + error.message);
    }
  }

  async switchConfig(fullPath, displayName) {
    try {
      await ConfigService.switchConfig(fullPath);
      toast(I18nService.t("config.toast.switch_success") + displayName);
      // 更新当前配置缓存
      this._cachedCurrentConfig = fullPath;
      // 强制重新渲染当前 tab 以更新"当前"标记
      if (this._selectedTab) {
        await this.renderActiveTab(this._selectedTab);
      }
      await this.ui.statusPage.update();
    } catch (error) {
      toast(I18nService.t("config.toast.switch_failed") + error.message);
    }
  }

  // ===================== 订阅管理 =====================

  async updateSubscription(dirName, displayName) {
    toast(I18nService.t("config.toast.updating_sub"));

    // 使用 setTimeout 让浏览器先渲染 UI
    setTimeout(async () => {
      try {
        // subscription.sh 期望传入的是订阅名称（不带 sub_ 前缀）
        await ConfigService.updateSubscription(displayName);
        toast(I18nService.t("config.toast.sub_updated"));
        // 清除该分组的缓存，强制重新加载
        this._cachedConfigInfos.delete(displayName);
        this.update();
      } catch (error) {
        toast(I18nService.t("config.toast.update_failed") + error.message);
        this.update();
      }
    }, 50);
  }

  async deleteSubscription(dirName, displayName) {
    try {
      const confirmed = await this.ui.confirm(
        I18nService.t("config.confirm.delete_sub", { name: displayName }),
      );
      if (!confirmed) return;

      // subscription.sh 期望传入的是订阅名称（不带 sub_ 前缀）
      await ConfigService.removeSubscription(displayName);
      toast(I18nService.t("config.toast.sub_deleted"));
      // 清除缓存，强制刷新分组列表
      this._cachedGroups = null;
      this._cachedConfigInfos.clear();
      await this.update(true);
    } catch (error: any) {
      toast(I18nService.t("config.toast.delete_failed") + error.message);
    }
  }

  async addSubscription(): Promise<void> {
    const dialog = document.getElementById("subscription-dialog") as any;
    const nameInput = document.getElementById(
      "subscription-name",
    ) as HTMLInputElement | null;
    const urlInput = document.getElementById(
      "subscription-url",
    ) as HTMLInputElement | null;

    if (nameInput) nameInput.value = "";
    if (urlInput) urlInput.value = "";
    if (dialog) dialog.open = true;
  }

  async saveSubscription(): Promise<void> {
    const nameInput = document.getElementById(
      "subscription-name",
    ) as HTMLInputElement | null;
    const urlInput = document.getElementById(
      "subscription-url",
    ) as HTMLInputElement | null;
    const saveBtn = document.getElementById("subscription-save");
    const cancelBtn = document.getElementById("subscription-cancel");
    const dialog = document.getElementById("subscription-dialog") as any;

    if (!nameInput || !urlInput) return;

    const name = nameInput.value.trim();
    const url = urlInput.value.trim();

    if (!name) {
      toast(I18nService.t("config.toast.enter_sub_name"));
      return;
    }

    if (!url) {
      toast(I18nService.t("config.toast.enter_sub_url"));
      return;
    }

    // 关闭对话框
    if (dialog) dialog.open = false;

    // 清空输入
    nameInput.value = "";
    urlInput.value = "";

    toast(I18nService.t("config.toast.downloading_sub"));

    // 使用 setTimeout 让浏览器先渲染 UI，再执行阻塞操作
    setTimeout(async () => {
      try {
        await ConfigService.addSubscription(name, url);
        toast(I18nService.t("config.toast.sub_added"));
        // 清除缓存，强制刷新分组列表
        this._cachedGroups = null;
        this._cachedConfigInfos.clear();
        await this.update(true);
      } catch (error: any) {
        toast(I18nService.t("config.toast.add_failed") + error.message);
      }
    }, 50);
  }

  // ===================== 原有方法 =====================

  async showDialog(filename: string | null = null): Promise<void> {
    const dialog = document.getElementById("config-dialog") as any;
    const filenameInput = document.getElementById(
      "config-filename",
    ) as HTMLInputElement | null;
    const contentInput = document.getElementById(
      "config-content",
    ) as HTMLTextAreaElement | null;

    if (!filenameInput || !contentInput) return;

    if (filename) {
      filenameInput.value = filename;
      filenameInput.disabled = true;
      const content = await ConfigService.readConfig(filename);
      contentInput.value = content;
    } else {
      filenameInput.value = "";
      filenameInput.disabled = false;
      contentInput.value = JSON.stringify(
        {
          outbounds: [
            {
              protocol: "vless",
              tag: "proxy",
              settings: {
                vnext: [{ address: "", port: 443, users: [{ id: "" }] }],
              },
            },
            { protocol: "freedom", tag: "direct" },
            { protocol: "blackhole", tag: "block" },
          ],
        },
        null,
        2,
      );
    }

    if (dialog) dialog.open = true;
  }

  async saveConfig(): Promise<void> {
    const filenameInput = document.getElementById(
      "config-filename",
    ) as HTMLInputElement | null;
    const contentInput = document.getElementById(
      "config-content",
    ) as HTMLTextAreaElement | null;

    if (!filenameInput || !contentInput) return;

    const filename = filenameInput.value.trim();
    const content = contentInput.value;

    if (!filename) {
      toast(I18nService.t("config.toast.enter_filename"));
      return;
    }

    if (!filename.endsWith(".json")) {
      toast(I18nService.t("config.toast.filename_json"));
      return;
    }

    try {
      JSON.parse(content);
      await ConfigService.saveConfig(filename, content);
      toast(I18nService.t("config.toast.save_success"));
      const dialog = document.getElementById("config-dialog") as any;
      if (dialog) dialog.open = false;
      this.update();
    } catch (error: any) {
      toast(I18nService.t("config.toast.save_failed") + error.message);
    }
  }

  async importNodeLink(): Promise<void> {
    const input = document.getElementById(
      "node-link-input",
    ) as HTMLInputElement | null;
    if (!input) return;

    const nodeLink = input.value.trim();

    if (!nodeLink) {
      toast(I18nService.t("config.toast.enter_link"));
      return;
    }

    const supportedProtocols = [
      "vless://",
      "vmess://",
      "trojan://",
      "ss://",
      "socks://",
      "http://",
      "https://",
      "hysteria2://",
      "hy2://",
    ];
    const isValid = supportedProtocols.some((protocol) =>
      nodeLink.startsWith(protocol),
    );

    if (!isValid) {
      toast(I18nService.t("config.toast.unsupported_link"));
      return;
    }

    try {
      const result = await ConfigService.importFromNodeLink(nodeLink);

      if (result.success) {
        toast(I18nService.t("config.toast.import_success"));
        const dialog = document.getElementById("node-link-dialog") as any;
        if (dialog) dialog.open = false;
        input.value = "";
        // 强制刷新配置列表
        this._cachedGroups = null;
        this._cachedConfigInfos.clear();
        await this.update(true);
      } else {
        toast(
          I18nService.t("config.toast.import_failed") +
            (result.error || I18nService.t("common.unknown")),
        );
      }
    } catch (error: any) {
      toast(I18nService.t("config.toast.import_failed") + error.message);
    }
  }
}
