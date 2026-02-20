import { toast } from "../utils/toast.js";
import { SettingsService } from "../services/settings-service.js";
import { I18nService } from "../i18n/i18n-service.js";
import { setColorScheme } from "mdui/functions/setColorScheme.js";
import { setTheme } from "mdui/functions/setTheme.js";
import type { UI } from "./ui-core.js";
import Sortable from "sortablejs";

const logoUrl = "/logo.png";

interface RoutingRule {
  name?: string;
  type: string;
  domain?: string;
  ip?: string;
  port?: string;
  protocol?: string;
  network?: string;
  outboundTag: string;
  enabled: boolean;
}

interface DnsServer {
  address: string;
  domains?: string[];
  expectIPs?: string[];
  skipFallback?: boolean;
  tag?: string;
}

interface DnsConfig {
  dns: {
    hosts: Record<string, string | string[]>;
    servers: (string | DnsServer)[];
  };
}

export class SettingsPageManager {
  ui: UI;
  routingRules: RoutingRule[];
  editingRuleIndex: number;
  dnsConfig: DnsConfig;
  editingServerIndex: number;
  editingHostKey: string | null;
  draggedIndex: number | null;
  sortable: Sortable | null;
  // Logs related
  _logsSelectedTab: string;
  _logsAutoRefreshEnabled: boolean;
  _logsAutoRefreshInterval: ReturnType<typeof setInterval> | null;
  _logsAutoRefreshMs: number;
  lastAppliedThemeMode: string;

  constructor(ui: UI) {
    this.ui = ui;
    this.routingRules = [];
    this.editingRuleIndex = -1;
    this.dnsConfig = { dns: { hosts: {}, servers: [] } };
    this.editingServerIndex = -1;
    this.editingHostKey = null;
    this.draggedIndex = null;
    this.sortable = null;
    // Logs related
    this._logsSelectedTab = "service";
    this._logsAutoRefreshEnabled = false;
    this._logsAutoRefreshInterval = null;
    this._logsAutoRefreshMs = 3000;

    this.setupEventListeners();
    this.setupRoutingRulesPage();
    this.setupProxySettingsPage();
    this.setupThemePage();
    this.setupLanguagePage();
    this.setupDnsPage();
    this.setupLogsPage();
    this.applyStoredTheme();
  }

  init(): void {
    // Initial setup
  }

  async update(): Promise<void> {
    // Update logic for main settings page if needed
    // Currently most settings are loaded on demand when clicking sub-items
  }

  setupEventListeners(): void {
    // 日志入口
    const logsEntry = document.getElementById("settings-logs-entry");
    if (logsEntry) {
      logsEntry.addEventListener("click", () => {
        this.ui.switchPage("logs");
      });
    }

    // 日志页返回按钮
    const logsBackBtn = document.getElementById("logs-back-btn");
    if (logsBackBtn) {
      logsBackBtn.addEventListener("click", () => {
        this.ui.switchPage("settings");
      });
    }

    // 路由设置入口
    const routingEntry = document.getElementById("settings-routing-entry");
    if (routingEntry) {
      routingEntry.addEventListener("click", () => {
        this.ui.switchPage("routing");
        this.loadRoutingRules();
      });
    }

    // 代理设置入口
    const proxyEntry = document.getElementById("settings-proxy-entry");
    if (proxyEntry) {
      proxyEntry.addEventListener("click", () => {
        this.ui.switchPage("proxy-settings");
        this.loadProxySettings();
      });
    }

    // 模块设置入口
    const moduleEntry = document.getElementById("settings-module-entry");
    if (moduleEntry) {
      moduleEntry.addEventListener("click", () => {
        this.ui.switchPage("module");
        this.loadModuleSettings();
      });
    }

    // 模块设置页返回按钮
    const moduleBackBtn = document.getElementById("module-back-btn");
    if (moduleBackBtn) {
      moduleBackBtn.addEventListener("click", () => {
        this.ui.switchPage("settings");
      });
    }

    // 模块设置开关
    const autoStartSwitch = document.getElementById("module-auto-start");
    if (autoStartSwitch) {
      autoStartSwitch.addEventListener("change", async (e) => {
        const target = e.target as HTMLInputElement;
        try {
          await SettingsService.setModuleSetting("AUTO_START", target.checked);
          toast(
            I18nService.t("settings.module.toast_autostart") +
              (target.checked
                ? I18nService.t("common.enabled")
                : I18nService.t("common.disabled")),
          );
        } catch (error: any) {
          toast(I18nService.t("common.set_failed") + error.message, true);
          target.checked = !target.checked;
        }
      });
    }

    const oneplusFixSwitch = document.getElementById("module-oneplus-fix");
    if (oneplusFixSwitch) {
      oneplusFixSwitch.addEventListener("change", async (e) => {
        const target = e.target as HTMLInputElement;
        try {
          await SettingsService.setModuleSetting(
            "ONEPLUS_A16_FIX",
            target.checked,
          );

          // 如果启用，立即执行修复脚本
          if (target.checked) {
            await SettingsService.executeOneplusFix();
            toast(I18nService.t("settings.module.toast_oneplus"));
          } else {
            toast(I18nService.t("settings.module.toast_oneplus_disabled"));
          }
        } catch (error: any) {
          toast(I18nService.t("common.set_failed") + error.message, true);
          target.checked = !target.checked;
        }
      });
    }

    // 主题设置入口
    const themeEntry = document.getElementById("settings-theme");
    if (themeEntry) {
      themeEntry.addEventListener("click", () => {
        this.ui.switchPage("theme");
        this.loadThemeSettings();
      });
    }

    // 语言设置入口
    const languageEntry = document.getElementById("settings-language");
    if (languageEntry) {
      languageEntry.addEventListener("click", () => {
        this.ui.switchPage("language");
        this.loadLanguageSettings();
      });
    }

    // DNS 设置入口
    const dnsEntry = document.getElementById("settings-dns-entry");
    if (dnsEntry) {
      dnsEntry.addEventListener("click", () => {
        this.ui.switchPage("dns");
        this.loadDnsConfig();
      });
    }

    // 关于
    const aboutEntry = document.getElementById("settings-about");
    if (aboutEntry) {
      aboutEntry.addEventListener("click", () => {
        this.showAboutDialog();
      });
    }
  }

  // ===================== 路由规则管理 =====================

  setupRoutingRulesPage() {
    // 返回按钮
    const backBtn = document.getElementById("routing-back-btn");
    if (backBtn) {
      backBtn.addEventListener("click", () => {
        this.ui.switchPage("settings");
      });
    }

    // 添加规则按钮
    const addBtn = document.getElementById("add-routing-rule-btn");
    if (addBtn) {
      addBtn.addEventListener("click", () => {
        this.showRuleDialog();
      });
    }

    // 规则对话框事件
    const cancelBtn = document.getElementById("rule-cancel");
    if (cancelBtn) {
      cancelBtn.addEventListener("click", () => {
        (document.getElementById("routing-rule-dialog") as any).open = false;
      });
    }

    const saveBtn = document.getElementById("rule-save");
    if (saveBtn) {
      saveBtn.addEventListener("click", () => {
        this.saveRule();
      });
    }

    // Clash 规则导入按钮
    const importClashBtn = document.getElementById("import-clash-rules-btn");
    if (importClashBtn) {
      importClashBtn.addEventListener("click", () => {
        this.showClashImportDialog();
      });
    }

    // Clash 导入对话框事件
    const clashCancelBtn = document.getElementById("clash-import-cancel");
    if (clashCancelBtn) {
      clashCancelBtn.addEventListener("click", () => {
        (document.getElementById("clash-import-dialog") as any).open = false;
      });
    }

    const clashConfirmBtn = document.getElementById("clash-import-confirm");
    if (clashConfirmBtn) {
      clashConfirmBtn.addEventListener("click", () => {
        this.importClashRules();
      });
    }

    // 初始化拖拽排序
    const listEl = document.getElementById("routing-rules-list");
    if (listEl) {
      this.sortable = new Sortable(listEl, {
        animation: 200,
        handle: ".drag-handle", // 仅允许通过手柄拖拽
        ghostClass: "sortable-ghost",
        dragClass: "sortable-drag",
        forceFallback: true,
        fallbackClass: "sortable-drag",
        fallbackOnBody: true,
        onEnd: (evt) => {
          const { oldIndex, newIndex } = evt;
          if (
            oldIndex !== undefined &&
            newIndex !== undefined &&
            oldIndex !== newIndex
          ) {
            this.moveRule(oldIndex, newIndex);
          }
        },
      });
    }
  }

  async loadRoutingRules() {
    try {
      this.routingRules = await SettingsService.getRoutingRules();
      this.renderRoutingRules();
    } catch (error) {
      console.error("加载路由规则失败:", error);
      toast(I18nService.t("settings.routing.toast_load_failed"));
    }
  }

  renderRoutingRules() {
    const listEl = document.getElementById("routing-rules-list");
    if (!listEl) return;

    listEl.innerHTML = "";

    if (this.routingRules.length === 0) {
      listEl.innerHTML = `
                <mdui-list-item>
                    <span slot="description">${I18nService.t("settings.routing.empty")}</span>
                </mdui-list-item>
            `;
      return;
    }

    this.routingRules.forEach((rule, index) => {
      const item = document.createElement("mdui-list-item");
      item.setAttribute("data-index", String(index));

      const parts = [];
      if (rule.domain)
        parts.push(
          `${I18nService.t("settings.routing.domain")}: ${rule.domain}`,
        );
      if (rule.ip)
        parts.push(`${I18nService.t("settings.routing.ip")}: ${rule.ip}`);
      if (rule.port)
        parts.push(`${I18nService.t("settings.routing.port")}: ${rule.port}`);
      if (rule.network)
        parts.push(
          `${I18nService.t("settings.routing.network")}: ${rule.network}`,
        );
      if (rule.protocol)
        parts.push(
          `${I18nService.t("settings.routing.protocol")}: ${rule.protocol}`,
        );

      const description =
        parts.length > 0
          ? parts.join(" | ")
          : I18nService.t("settings.routing.unconditional");
      const outboundLabel =
        {
          proxy: I18nService.t("settings.routing.outbound_proxy"),
          direct: I18nService.t("settings.routing.outbound_direct"),
          block: I18nService.t("settings.routing.outbound_block"),
        }[rule.outboundTag] || rule.outboundTag;

      // 拖拽手柄
      const dragHandle = document.createElement("mdui-icon");
      dragHandle.setAttribute("slot", "icon");
      dragHandle.setAttribute("name", "drag_indicator");
      dragHandle.classList.add("drag-handle"); // 添加标识类
      dragHandle.style.cssText =
        "cursor: grab; color: var(--mdui-color-on-surface-variant); touch-action: none;"; // touch-action: none 对 Sortable 很重要
      item.appendChild(dragHandle);

      item.setAttribute(
        "headline",
        rule.name ||
          `${I18nService.t("settings.routing.rule_prefix")}${index + 1}`,
      );

      // 使用 description slot 显示详情和出站
      const descDiv = document.createElement("div");
      descDiv.slot = "description";
      descDiv.style.cssText =
        "display: flex; justify-content: space-between; width: 100%;";

      const descSpan = document.createElement("span");
      descSpan.textContent = description;
      descSpan.style.cssText =
        "overflow: hidden; text-overflow: ellipsis; white-space: nowrap; flex: 1;";

      const outboundSpan = document.createElement("span");
      outboundSpan.textContent = outboundLabel;
      outboundSpan.style.cssText =
        "margin-left: 8px; padding: 2px 6px; border-radius: 4px; font-size: 11px; background: var(--mdui-color-secondary-container); color: var(--mdui-color-on-secondary-container);";

      descDiv.appendChild(descSpan);
      descDiv.appendChild(outboundSpan);
      item.appendChild(descDiv);

      // 右侧容器：开关和菜单
      const endContainer = document.createElement("div");
      endContainer.slot = "end-icon";
      endContainer.style.cssText =
        "display: flex; align-items: center; gap: 4px;";

      // 启用开关
      const switchEl = document.createElement("mdui-switch") as any;
      switchEl.checked = rule.enabled !== false;
      switchEl.addEventListener("change", async (e: Event) => {
        e.stopPropagation();
        rule.enabled = (e.target as HTMLInputElement).checked;
        await this.saveRulesToBackend();
      });
      endContainer.appendChild(switchEl);

      // 菜单
      const dropdown = document.createElement("mdui-dropdown");
      dropdown.setAttribute("placement", "bottom-end");

      const menuBtn = document.createElement("mdui-button-icon");
      menuBtn.setAttribute("slot", "trigger");
      menuBtn.setAttribute("icon", "more_vert");
      menuBtn.addEventListener("click", (e) => e.stopPropagation());
      dropdown.appendChild(menuBtn);

      const menu = document.createElement("mdui-menu");

      // 编辑
      const editItem = document.createElement("mdui-menu-item");
      editItem.innerHTML = `<mdui-icon slot="icon" name="edit"></mdui-icon>${I18nService.t("common.edit")}`;
      editItem.addEventListener("click", (e) => {
        e.stopPropagation();
        (dropdown as any).open = false;
        this.showRuleDialog(rule, index);
      });
      menu.appendChild(editItem);

      // 删除
      const deleteItem = document.createElement("mdui-menu-item");
      deleteItem.innerHTML = `<mdui-icon slot="icon" name="delete"></mdui-icon>${I18nService.t("common.delete")}`;
      deleteItem.style.color = "var(--mdui-color-error)";
      deleteItem.addEventListener("click", async (e) => {
        e.stopPropagation();
        (dropdown as any).open = false;
        if (
          await this.ui.confirm(
            I18nService.t("settings.routing.confirm_delete", {
              name:
                rule.name ||
                `${I18nService.t("settings.routing.rule_prefix")}${index + 1}`,
            }),
          )
        ) {
          this.routingRules.splice(index, 1);
          await this.saveRulesToBackend();
          this.renderRoutingRules();
        }
      });
      menu.appendChild(deleteItem);

      dropdown.appendChild(menu);
      endContainer.appendChild(dropdown);
      item.appendChild(endContainer);

      listEl.appendChild(item);
    });
  }

  // 移动规则
  async moveRule(fromIndex, toIndex) {
    const [movedRule] = this.routingRules.splice(fromIndex, 1);
    this.routingRules.splice(toIndex, 0, movedRule);
    await this.saveRulesToBackend();
    this.renderRoutingRules();
    toast(I18nService.t("routing.toast_reordered"));
  }

  showRuleDialog(rule: RoutingRule | null = null, index = -1): void {
    this.editingRuleIndex = index;
    const dialog = document.getElementById("routing-rule-dialog") as any;

    // 设置标题
    dialog.headline = rule
      ? I18nService.t("settings.routing.dialog_edit")
      : I18nService.t("settings.routing.dialog_add");

    // 填充表单
    (document.getElementById("rule-name") as HTMLInputElement).value =
      rule?.name || "";
    (document.getElementById("rule-domain") as HTMLInputElement).value =
      rule?.domain || "";
    (document.getElementById("rule-ip") as HTMLInputElement).value =
      rule?.ip || "";
    (document.getElementById("rule-port") as HTMLInputElement).value =
      rule?.port || "";
    (document.getElementById("rule-protocol") as HTMLInputElement).value =
      rule?.protocol || "";
    (document.getElementById("rule-network") as HTMLInputElement).value =
      rule?.network || "";
    (document.getElementById("rule-outbound") as HTMLInputElement).value =
      rule?.outboundTag || "proxy";

    dialog.open = true;
  }

  async saveRule(): Promise<void> {
    const name = (
      document.getElementById("rule-name") as HTMLInputElement
    ).value.trim();
    const domain = (
      document.getElementById("rule-domain") as HTMLInputElement
    ).value.trim();
    const ip = (
      document.getElementById("rule-ip") as HTMLInputElement
    ).value.trim();
    const port = (
      document.getElementById("rule-port") as HTMLInputElement
    ).value.trim();
    const protocol = (
      document.getElementById("rule-protocol") as HTMLInputElement
    ).value.trim();
    const network = (
      document.getElementById("rule-network") as HTMLInputElement
    ).value.trim();
    const outboundTag = (
      document.getElementById("rule-outbound") as HTMLInputElement
    ).value;

    // 验证
    if (!domain && !ip && !port && !protocol && !network) {
      toast(I18nService.t("settings.routing.toast_input_condition"));
      return;
    }

    const rule: RoutingRule = {
      name:
        name ||
        (this.editingRuleIndex >= 0
          ? `${I18nService.t("settings.routing.rule_prefix")}${this.editingRuleIndex + 1}`
          : `${I18nService.t("settings.routing.rule_prefix")}${this.routingRules.length + 1}`),
      type: "field",
      domain,
      ip,
      port,
      protocol,
      network,
      outboundTag,
      enabled: true,
    };

    if (this.editingRuleIndex >= 0) {
      // 保留原有的 enabled 状态
      rule.enabled = this.routingRules[this.editingRuleIndex].enabled !== false;
      this.routingRules[this.editingRuleIndex] = rule;
    } else {
      this.routingRules.push(rule);
    }

    await this.saveRulesToBackend();
    this.renderRoutingRules();
    (document.getElementById("routing-rule-dialog") as any).open = false;
    toast(
      this.editingRuleIndex >= 0
        ? I18nService.t("settings.routing.toast_updated")
        : I18nService.t("settings.routing.toast_added"),
    );
  }

  async saveRulesToBackend() {
    try {
      await SettingsService.saveRoutingRules(this.routingRules);
      await SettingsService.applyRoutingRules(this.routingRules);
    } catch (error) {
      console.error("保存规则失败:", error);
      toast(I18nService.t("common.save_failed") + error.message);
    }
  }

  // Clash 规则导入对话框
  showClashImportDialog(): void {
    const dialog = document.getElementById("clash-import-dialog") as any;
    (document.getElementById("clash-rule-name") as HTMLInputElement).value = "";
    (document.getElementById("clash-rule-url") as HTMLInputElement).value = "";
    (document.getElementById("clash-rule-outbound") as HTMLInputElement).value =
      "block";
    dialog.open = true;
  }

  // 导入 Clash 规则
  async importClashRules(): Promise<void> {
    const name = (
      document.getElementById("clash-rule-name") as HTMLInputElement
    ).value.trim();
    const url = (
      document.getElementById("clash-rule-url") as HTMLInputElement
    ).value.trim();
    const outboundTag = (
      document.getElementById("clash-rule-outbound") as HTMLInputElement
    ).value;

    if (!url) {
      toast(I18nService.t("routing.toast_url_required"));
      return;
    }

    try {
      toast(I18nService.t("routing.toast_importing"));

      // 使用 Service 层获取并解析域名列表
      const domains = await SettingsService.importClashRulesFromUrl(url);

      if (domains.length === 0) {
        toast(I18nService.t("routing.toast_no_domains"));
        return;
      }

      // 创建路由规则
      const rule: RoutingRule = {
        name: name || `Clash 规则 (${domains.length} 条)`,
        type: "field",
        domain: domains.join(","),
        ip: "",
        port: "",
        protocol: "",
        network: "",
        outboundTag: outboundTag,
        enabled: true,
      };

      this.routingRules.push(rule);
      await this.saveRulesToBackend();
      this.renderRoutingRules();

      (document.getElementById("clash-import-dialog") as any).open = false;
      toast(
        I18nService.t("routing.toast_imported", {
          count: String(domains.length),
        }),
      );
    } catch (error: any) {
      console.error("导入 Clash 规则失败:", error);
      toast(I18nService.t("routing.toast_import_failed") + error.message);
    }
  }

  // ===================== DNS 设置页面 =====================

  setupDnsPage() {
    // 返回按钮
    const backBtn = document.getElementById("dns-back-btn");
    if (backBtn) {
      backBtn.addEventListener("click", () => {
        this.ui.switchPage("settings");
      });
    }

    // 添加服务器按钮
    const addServerBtn = document.getElementById("add-dns-server-btn");
    if (addServerBtn) {
      addServerBtn.addEventListener("click", () => {
        this.showServerDialog();
      });
    }

    // 添加 Host 按钮
    const addHostBtn = document.getElementById("add-dns-host-btn");
    if (addHostBtn) {
      addHostBtn.addEventListener("click", () => {
        this.showHostDialog();
      });
    }

    // 服务器对话框事件
    const serverCancelBtn = document.getElementById("dns-server-cancel");
    if (serverCancelBtn) {
      serverCancelBtn.addEventListener("click", () => {
        const dialog = document.getElementById("dns-server-dialog") as any;
        if (dialog) dialog.open = false;
      });
    }

    const serverSaveBtn = document.getElementById("dns-server-save");
    if (serverSaveBtn) {
      serverSaveBtn.addEventListener("click", () => {
        this.saveServer();
      });
    }

    // Host 对话框事件
    const hostCancelBtn = document.getElementById("dns-host-cancel");
    if (hostCancelBtn) {
      hostCancelBtn.addEventListener("click", () => {
        (document.getElementById("dns-host-dialog") as any).open = false;
      });
    }

    const hostSaveBtn = document.getElementById("dns-host-save");
    if (hostSaveBtn) {
      hostSaveBtn.addEventListener("click", () => {
        this.saveHost();
      });
    }
  }

  async loadDnsConfig() {
    try {
      this.dnsConfig = await SettingsService.getDnsConfig();
      this.renderDnsServers();
      this.renderDnsHosts();
    } catch (error) {
      console.error("加载 DNS 配置失败:", error);
      toast(I18nService.t("settings.dns.toast_load_failed"));
    }
  }

  renderDnsServers() {
    const listEl = document.getElementById("dns-servers-list");
    if (!listEl) return;

    const servers = this.dnsConfig.dns?.servers || [];
    listEl.innerHTML = "";

    if (servers.length === 0) {
      listEl.innerHTML = `
                <mdui-list-item>
                    <span slot="description">${I18nService.t("settings.dns.empty_server")}</span>
                </mdui-list-item>
            `;
      return;
    }

    servers.forEach((server, index) => {
      const item = document.createElement("mdui-list-item");
      const isSimple = typeof server === "string";
      const address = isSimple ? server : server.address;
      const domains = isSimple ? [] : server.domains || [];
      const tag = isSimple ? "" : server.tag || "";

      item.setAttribute("headline", address);

      const descParts = [];
      if (domains.length > 0)
        descParts.push(
          `${I18nService.t("settings.routing.domain")}: ${domains.slice(0, 2).join(", ")}${domains.length > 2 ? "..." : ""}`,
        );
      if (tag)
        descParts.push(`${I18nService.t("settings.dns.label_tag")}: ${tag}`);

      if (descParts.length > 0) {
        const descSpan = document.createElement("span");
        descSpan.slot = "description";
        descSpan.textContent = descParts.join(" | ");
        item.appendChild(descSpan);
      }

      // 编辑/删除操作
      const endContainer = document.createElement("div");
      endContainer.slot = "end-icon";
      endContainer.style.cssText =
        "display: flex; align-items: center; gap: 4px;";

      const dropdown = document.createElement("mdui-dropdown");
      dropdown.setAttribute("placement", "bottom-end");

      const menuBtn = document.createElement("mdui-button-icon");
      menuBtn.setAttribute("slot", "trigger");
      menuBtn.setAttribute("icon", "more_vert");
      menuBtn.addEventListener("click", (e) => e.stopPropagation());
      dropdown.appendChild(menuBtn);

      const menu = document.createElement("mdui-menu");

      const editItem = document.createElement("mdui-menu-item");
      editItem.innerHTML = `<mdui-icon slot="icon" name="edit"></mdui-icon>${I18nService.t("common.edit")}`;
      editItem.addEventListener("click", (e) => {
        e.stopPropagation();
        dropdown.open = false;
        this.showServerDialog(server, index);
      });
      menu.appendChild(editItem);

      const deleteItem = document.createElement("mdui-menu-item");
      deleteItem.innerHTML = `<mdui-icon slot="icon" name="delete"></mdui-icon>${I18nService.t("common.delete")}`;
      deleteItem.style.color = "var(--mdui-color-error)";
      deleteItem.addEventListener("click", async (e) => {
        e.stopPropagation();
        dropdown.open = false;
        if (
          await this.ui.confirm(
            I18nService.t("settings.dns.confirm_delete_server", {
              address: address,
            }),
          )
        ) {
          this.dnsConfig.dns.servers.splice(index, 1);
          await this.saveDnsToBackend();
          this.renderDnsServers();
        }
      });
      menu.appendChild(deleteItem);

      dropdown.appendChild(menu);
      endContainer.appendChild(dropdown);
      item.appendChild(endContainer);

      listEl.appendChild(item);
    });
  }

  renderDnsHosts() {
    const listEl = document.getElementById("dns-hosts-list");
    if (!listEl) return;

    const hosts = this.dnsConfig.dns?.hosts || {};
    const hostKeys = Object.keys(hosts);
    listEl.innerHTML = "";

    if (hostKeys.length === 0) {
      listEl.innerHTML = `
                <mdui-list-item>
                    <span slot="description">${I18nService.t("settings.dns.empty_host")}</span>
                </mdui-list-item>
            `;
      return;
    }

    hostKeys.forEach((domain) => {
      const item = document.createElement("mdui-list-item");
      const value = hosts[domain];
      const ips = Array.isArray(value) ? value : [value];

      item.setAttribute("headline", domain);

      const descSpan = document.createElement("span");
      descSpan.slot = "description";
      descSpan.textContent =
        ips.slice(0, 3).join(", ") + (ips.length > 3 ? "..." : "");
      item.appendChild(descSpan);

      // 编辑/删除操作
      const endContainer = document.createElement("div");
      endContainer.slot = "end-icon";
      endContainer.style.cssText =
        "display: flex; align-items: center; gap: 4px;";

      const dropdown = document.createElement("mdui-dropdown");
      dropdown.setAttribute("placement", "bottom-end");

      const menuBtn = document.createElement("mdui-button-icon");
      menuBtn.setAttribute("slot", "trigger");
      menuBtn.setAttribute("icon", "more_vert");
      menuBtn.addEventListener("click", (e) => e.stopPropagation());
      dropdown.appendChild(menuBtn);

      const menu = document.createElement("mdui-menu");

      const editItem = document.createElement("mdui-menu-item");
      editItem.innerHTML = `<mdui-icon slot="icon" name="edit"></mdui-icon>${I18nService.t("common.edit")}`;
      editItem.addEventListener("click", (e) => {
        e.stopPropagation();
        dropdown.open = false;
        this.showHostDialog(domain, value);
      });
      menu.appendChild(editItem);

      const deleteItem = document.createElement("mdui-menu-item");
      deleteItem.innerHTML = `<mdui-icon slot="icon" name="delete"></mdui-icon>${I18nService.t("common.delete")}`;
      deleteItem.style.color = "var(--mdui-color-error)";
      deleteItem.addEventListener("click", async (e) => {
        e.stopPropagation();
        dropdown.open = false;
        if (
          await this.ui.confirm(
            I18nService.t("settings.dns.confirm_delete_host", {
              domain: domain,
            }),
          )
        ) {
          delete this.dnsConfig.dns.hosts[domain];
          await this.saveDnsToBackend();
          this.renderDnsHosts();
        }
      });
      menu.appendChild(deleteItem);

      dropdown.appendChild(menu);
      endContainer.appendChild(dropdown);
      item.appendChild(endContainer);

      listEl.appendChild(item);
    });
  }

  showServerDialog(server: string | DnsServer | null = null, index = -1): void {
    this.editingServerIndex = index;
    const dialog = document.getElementById("dns-server-dialog") as any;
    dialog.headline = server
      ? I18nService.t("settings.dns.server_dialog_edit")
      : I18nService.t("settings.dns.server_dialog_add");

    const isSimple = typeof server === "string";
    const address = server
      ? isSimple
        ? server
        : (server as DnsServer).address
      : "";
    const domains =
      server && !isSimple
        ? ((server as DnsServer).domains || []).join(", ")
        : "";
    const expectIPs =
      server && !isSimple
        ? ((server as DnsServer).expectIPs || []).join(", ")
        : "";
    const skipFallback =
      server && !isSimple ? !!(server as DnsServer).skipFallback : false;
    const tag = server && !isSimple ? (server as DnsServer).tag || "" : "";

    (document.getElementById("dns-server-address") as HTMLInputElement).value =
      address;
    (document.getElementById("dns-server-domains") as HTMLInputElement).value =
      domains;
    (
      document.getElementById("dns-server-expect-ips") as HTMLInputElement
    ).value = expectIPs;
    (
      document.getElementById("dns-server-skip-fallback") as HTMLInputElement
    ).checked = skipFallback;
    (document.getElementById("dns-server-tag") as HTMLInputElement).value = tag;

    dialog.open = true;
  }

  showHostDialog(
    domain: string | null = null,
    value: string | string[] | null = null,
  ): void {
    this.editingHostKey = domain;
    const dialog = document.getElementById("dns-host-dialog") as any;
    dialog.headline = domain
      ? I18nService.t("settings.dns.host_dialog_edit")
      : I18nService.t("settings.dns.host_dialog_add");

    const ips = value ? (Array.isArray(value) ? value.join(", ") : value) : "";

    (document.getElementById("dns-host-domain") as HTMLInputElement).value =
      domain || "";
    (document.getElementById("dns-host-ip") as HTMLInputElement).value = ips;

    dialog.open = true;
  }

  async saveServer(): Promise<void> {
    const address = (
      document.getElementById("dns-server-address") as HTMLInputElement
    ).value.trim();
    const domainsStr = (
      document.getElementById("dns-server-domains") as HTMLInputElement
    ).value.trim();
    const expectIPsStr = (
      document.getElementById("dns-server-expect-ips") as HTMLInputElement
    ).value.trim();
    const skipFallback = (
      document.getElementById("dns-server-skip-fallback") as HTMLInputElement
    ).checked;
    const tag = (
      document.getElementById("dns-server-tag") as HTMLInputElement
    ).value.trim();

    if (!address) {
      toast(I18nService.t("settings.dns.toast_enter_address"));
      return;
    }

    const domains = domainsStr
      ? domainsStr
          .split(",")
          .map((d) => d.trim())
          .filter((d) => d)
      : [];
    const expectIPs = expectIPsStr
      ? expectIPsStr
          .split(",")
          .map((i) => i.trim())
          .filter((i) => i)
      : [];

    let server: string | DnsServer;
    if (!domains.length && !expectIPs.length && !skipFallback && !tag) {
      server = address;
    } else {
      server = { address } as DnsServer;
      if (domains.length) (server as DnsServer).domains = domains;
      if (expectIPs.length) (server as DnsServer).expectIPs = expectIPs;
      if (skipFallback) (server as DnsServer).skipFallback = true;
      if (tag) (server as DnsServer).tag = tag;
    }

    if (!this.dnsConfig.dns) this.dnsConfig.dns = { hosts: {}, servers: [] };
    if (!this.dnsConfig.dns.servers) this.dnsConfig.dns.servers = [];

    if (this.editingServerIndex >= 0) {
      this.dnsConfig.dns.servers[this.editingServerIndex] = server;
    } else {
      this.dnsConfig.dns.servers.push(server);
    }

    await this.saveDnsToBackend();
    this.renderDnsServers();
    (document.getElementById("dns-server-dialog") as any).open = false;
    toast(
      this.editingServerIndex >= 0
        ? I18nService.t("settings.dns.toast_server_updated")
        : I18nService.t("settings.dns.toast_server_added"),
    );
  }

  async saveHost(): Promise<void> {
    const domain = (
      document.getElementById("dns-host-domain") as HTMLInputElement
    ).value.trim();
    const ipStr = (
      document.getElementById("dns-host-ip") as HTMLInputElement
    ).value.trim();

    if (!domain) {
      toast(I18nService.t("settings.dns.toast_enter_domain"));
      return;
    }
    if (!ipStr) {
      toast(I18nService.t("settings.dns.toast_enter_ip"));
      return;
    }

    const ips = ipStr
      .split(",")
      .map((i) => i.trim())
      .filter((i) => i);
    const value = ips.length === 1 ? ips[0] : ips;

    if (!this.dnsConfig.dns) this.dnsConfig.dns = { hosts: {}, servers: [] };
    if (!this.dnsConfig.dns.hosts) this.dnsConfig.dns.hosts = {};

    // 如果是编辑且域名改变了，删除旧的
    if (this.editingHostKey && this.editingHostKey !== domain) {
      delete this.dnsConfig.dns.hosts[this.editingHostKey];
    }

    this.dnsConfig.dns.hosts[domain] = value;

    await this.saveDnsToBackend();
    this.renderDnsHosts();
    (document.getElementById("dns-host-dialog") as any).open = false;
    toast(
      this.editingHostKey
        ? I18nService.t("settings.dns.toast_host_updated")
        : I18nService.t("settings.dns.toast_host_added"),
    );
  }

  async saveDnsToBackend() {
    try {
      await SettingsService.saveDnsConfig(this.dnsConfig);
    } catch (error) {
      console.error("保存 DNS 配置失败:", error);
      toast(I18nService.t("common.save_failed") + error.message);
    }
  }

  // ===================== 代理设置页面 =====================

  setupProxySettingsPage() {
    // 返回按钮
    const backBtn = document.getElementById("proxy-settings-back-btn");
    if (backBtn) {
      backBtn.addEventListener("click", () => {
        this.ui.switchPage("settings");
      });
    }

    // 保存按钮
    const saveBtn = document.getElementById("proxy-save-btn");
    if (saveBtn) {
      saveBtn.addEventListener("click", () => {
        this.saveProxySettings();
      });
    }
  }

  async loadProxySettings(): Promise<void> {
    try {
      const settings = await SettingsService.getProxySettings();

      // 设置值的辅助函数
      const setVal = (id: string, value: any) => {
        const el = document.getElementById(id) as HTMLInputElement | null;
        if (!el) return;
        if (
          el.tagName === "MDUI-SWITCH" ||
          (el.tagName === "INPUT" && el.type === "checkbox")
        ) {
          el.checked = !!value;
        } else {
          el.value = String(value === undefined ? "" : value);
        }
      };

      // 核心配置
      setVal("proxy-mode-settings", settings.proxy_mode);
      setVal("proxy-tcp-port", settings.proxy_tcp_port);
      setVal("proxy-udp-port", settings.proxy_udp_port);
      setVal("proxy-table-id", settings.table_id);
      setVal("proxy-routing-mark", settings.routing_mark);
      setVal("proxy-mark-value", settings.mark_value);
      setVal("proxy-mark-value6", settings.mark_value6);

      // 开关设置
      setVal("proxy-mobile", settings.proxy_mobile);
      setVal("proxy-wifi", settings.proxy_wifi);
      setVal("proxy-hotspot", settings.proxy_hotspot);
      setVal("proxy-usb", settings.proxy_usb);
      setVal("proxy-tcp", settings.proxy_tcp);
      setVal("proxy-udp", settings.proxy_udp);
      setVal("proxy-ipv6", settings.proxy_ipv6);
      setVal("proxy-force-mark-bypass", settings.force_mark_bypass);
      setVal("proxy-block-quic", settings.block_quic);
      setVal("proxy-performance-mode", settings.compatibility_mode);

      // 接口设置
      setVal("proxy-mobile-int", settings.mobile_interface);
      setVal("proxy-wifi-int", settings.wifi_interface);
      setVal("proxy-hotspot-int", settings.hotspot_interface);
      setVal("proxy-usb-int", settings.usb_interface);
      setVal("proxy-other-proxy-int", settings.other_proxy_interfaces);
      setVal("proxy-other-bypass-int", settings.other_bypass_interfaces);

      // DNS 设置
      setVal("proxy-dns-hijack", settings.dns_hijack_enable);
      setVal("proxy-dns-port", settings.dns_port);

      // IP 列表
      setVal("proxy-bypass-cn", settings.bypass_cn_ip);
      setVal("proxy-cn-ip-file", settings.cn_ip_file);
      setVal("proxy-cn-ipv6-file", settings.cn_ipv6_file);
      setVal("proxy-cn-ip-url", settings.cn_ip_url);
      setVal("proxy-cn-ipv6-url", settings.cn_ipv6_url);
      setVal("proxy-bypass-v4", settings.bypass_ipv4_list);
      setVal("proxy-bypass-v6", settings.bypass_ipv6_list);
      setVal("proxy-proxy-v4", settings.proxy_ipv4_list);
      setVal("proxy-proxy-v6", settings.proxy_ipv6_list);

      // MAC 过滤
      setVal("proxy-mac-enable", settings.mac_filter_enable);
      setVal("proxy-mac-mode", settings.mac_proxy_mode);
      setVal("proxy-proxy-macs", settings.proxy_macs_list);
      setVal("proxy-bypass-macs", settings.bypass_macs_list);
    } catch (error) {
      console.error("加载代理设置失败:", error);
      const container = document.getElementById("proxy-settings-page");
      if (container) {
        // simple error display, better toast
        toast(I18nService.t("common.load_failed"));
      }
    }
  }

  async saveProxySettings(): Promise<void> {
    try {
      const getVal = (
        id: string,
        type: "string" | "number" | "bool" = "string",
      ) => {
        const el = document.getElementById(id) as HTMLInputElement | null;
        if (!el) return undefined;
        if (type === "bool") return el.checked;
        const val = el.value;
        if (type === "number") return Number(val);
        return val;
      };

      const settings: any = {
        // 核心配置
        proxy_mode: getVal("proxy-mode-settings", "number"),
        proxy_tcp_port: getVal("proxy-tcp-port"),
        proxy_udp_port: getVal("proxy-udp-port"),
        table_id: getVal("proxy-table-id", "number"),
        routing_mark: getVal("proxy-routing-mark"),
        mark_value: getVal("proxy-mark-value", "number"),
        mark_value6: getVal("proxy-mark-value6", "number"),

        // 开关设置
        proxy_mobile: getVal("proxy-mobile", "bool"),
        proxy_wifi: getVal("proxy-wifi", "bool"),
        proxy_hotspot: getVal("proxy-hotspot", "bool"),
        proxy_usb: getVal("proxy-usb", "bool"),
        proxy_tcp: getVal("proxy-tcp", "bool"),
        proxy_udp: getVal("proxy-udp", "bool"),
        proxy_ipv6: getVal("proxy-ipv6", "bool"),
        force_mark_bypass: getVal("proxy-force-mark-bypass", "bool"),
        block_quic: getVal("proxy-block-quic", "bool"),
        compatibility_mode: getVal("proxy-performance-mode", "bool"),

        // 接口设置
        mobile_interface: getVal("proxy-mobile-int"),
        wifi_interface: getVal("proxy-wifi-int"),
        hotspot_interface: getVal("proxy-hotspot-int"),
        usb_interface: getVal("proxy-usb-int"),
        other_proxy_interfaces: getVal("proxy-other-proxy-int"),
        other_bypass_interfaces: getVal("proxy-other-bypass-int"),

        // DNS 设置
        dns_hijack_enable: getVal("proxy-dns-hijack", "bool"),
        dns_port: getVal("proxy-dns-port"),

        // IP 列表
        bypass_cn_ip: getVal("proxy-bypass-cn", "bool"),
        cn_ip_file: getVal("proxy-cn-ip-file"),
        cn_ipv6_file: getVal("proxy-cn-ipv6-file"),
        cn_ip_url: getVal("proxy-cn-ip-url"),
        cn_ipv6_url: getVal("proxy-cn-ipv6-url"),
        bypass_ipv4_list: getVal("proxy-bypass-v4"),
        bypass_ipv6_list: getVal("proxy-bypass-v6"),
        proxy_ipv4_list: getVal("proxy-proxy-v4"),
        proxy_ipv6_list: getVal("proxy-proxy-v6"),

        // MAC 过滤
        mac_filter_enable: getVal("proxy-mac-enable", "bool"),
        mac_proxy_mode: getVal("proxy-mac-mode"),
        proxy_macs_list: getVal("proxy-proxy-macs"),
        bypass_macs_list: getVal("proxy-bypass-macs"),
      };

      await SettingsService.saveProxySettings(settings);
      toast(I18nService.t("settings.proxy.save_success"));
      toast(I18nService.t("settings.proxy.save_warning"));
    } catch (error: any) {
      toast(I18nService.t("common.save_failed") + error.message, true);
    }
  }

  // ===================== 主题页面 =====================

  setupThemePage() {
    // 返回按钮
    const backBtn = document.getElementById("theme-back-btn");
    if (backBtn) {
      backBtn.addEventListener("click", () => {
        this.ui.switchPage("settings");
      });
    }

    // 模式选择
    const modeGroup = document.getElementById("theme-mode-group") as any;
    if (modeGroup) {
      modeGroup.addEventListener("change", (e: Event) => {
        const mode = (e.target as HTMLInputElement).value;
        this.applyThemeMode(mode);
      });
    }

    // 颜色选择
    const colorPalette = document.getElementById("color-palette");
    if (colorPalette) {
      colorPalette.addEventListener("click", (e: Event) => {
        const colorItem = (e.target as HTMLElement).closest(
          ".color-item",
        ) as HTMLElement | null;
        if (colorItem) {
          const color = colorItem.dataset.color;
          if (color) {
            this.applyThemeColor(color);
            this.updateColorSelection(color);
          }
        }
      });
    }

    // 莫奈取色开关
    this.setupMonetToggle();
  }

  loadThemeSettings(): void {
    const savedTheme = localStorage.getItem("theme") || "auto";
    const savedColor = localStorage.getItem("themeColor") || "#6750A4";
    this.lastAppliedThemeMode = savedTheme;

    // 设置模式选择
    const modeGroup = document.getElementById("theme-mode-group") as any;
    if (modeGroup) {
      modeGroup.value = savedTheme;
    }

    // 设置颜色选择
    this.updateColorSelection(savedColor);

    // 设置莫奈取色开关状态
    this.updateMonetToggleState();
  }

  updateColorSelection(selectedColor: string): void {
    const colorItems = document.querySelectorAll(".color-item");
    colorItems.forEach((item) => {
      if ((item as HTMLElement).dataset.color === selectedColor) {
        item.classList.add("selected");
      } else {
        item.classList.remove("selected");
      }
    });
  }

  applyThemeMode(mode) {
    if (mode === this.lastAppliedThemeMode) {
      return;
    }
    this.lastAppliedThemeMode = mode;
    localStorage.setItem("theme", mode);
    setTheme(mode);

    const savedColor = localStorage.getItem("themeColor") || "#6750A4";
    const html = document.documentElement;
    const isDark = window.matchMedia("(prefers-color-scheme: dark)").matches;

    if (mode === "light" || mode === "dark") {
      // 浅色或深色模式：使用用户选择的主题色
      const forceDark = mode === "dark";
      this.applyAllMonetVariables(savedColor, forceDark);
    } else {
      // 自动模式：根据莫奈取色设置处理
      const monetEnabled = localStorage.getItem("monetEnabled") === "true";
      if (monetEnabled) {
        // 莫奈取色开启：使用 KernelSU 注入的变量
        html.classList.add("mdui-theme-auto");
        html.classList.remove("mdui-theme-light", "mdui-theme-dark");
        this.removeAllMonetVariables();
        setColorScheme(savedColor);
      } else {
        // 莫奈取色关闭：使用用户选择的主题色
        html.classList.remove("mdui-theme-auto");
        html.classList.add(isDark ? "mdui-theme-dark" : "mdui-theme-light");
        this.applyAllMonetVariables(savedColor, isDark);
      }
    }

    this.updateMonetToggleState();
    const modeName =
      mode === "auto"
        ? I18nService.t("settings.theme.mode_auto")
        : mode === "light"
          ? I18nService.t("settings.theme.mode_light")
          : I18nService.t("settings.theme.mode_dark");
    toast(I18nService.t("settings.theme.toast_mode_switched") + modeName);

    this.ui.statusPage.updateSpeedChartColors();
  }

  applyThemeColor(color) {
    localStorage.setItem("themeColor", color);

    const monetEnabled = localStorage.getItem("monetEnabled") === "true";
    const savedTheme = localStorage.getItem("theme") || "auto";
    const isAutoMode = savedTheme === "auto";
    const isDark = window.matchMedia("(prefers-color-scheme: dark)").matches;

    // 非自动模式（浅色/深色），或者自动模式下莫奈取色关闭时，设置所有主题变量
    if (!isAutoMode) {
      const forceDark = savedTheme === "dark";
      this.applyAllMonetVariables(color, forceDark);
    } else if (!monetEnabled) {
      this.applyAllMonetVariables(color, isDark);
    } else {
      // 自动模式 + 莫奈取色开启：只更新 MDUI 的颜色方案
      setColorScheme(color);
    }

    this.updateMonetToggleState();
    toast(I18nService.t("settings.theme.toast_color_changed"));

    this.ui.statusPage.updateSpeedChartColors();
  }

  /**
   * 设置所有 Monet 主题变量（使用 MDUI 生成的值覆盖 monet.css 中的默认值）
   * @param {string} primaryColor - 主题主色调
   * @param {boolean} isDark - 是否为深色模式
   */
  applyAllMonetVariables(primaryColor, isDark) {
    const html = document.documentElement;

    // 调用 setColorScheme 让 MDUI 组件内部使用正确的颜色
    setColorScheme(primaryColor);

    // 解析主题色为 RGB
    const hexToRgb = (hex) => {
      const result = /^#?([a-f\d]{2})([a-f\d]{2})([a-f\d]{2})$/i.exec(hex);
      return result
        ? {
            r: parseInt(result[1], 16),
            g: parseInt(result[2], 16),
            b: parseInt(result[3], 16),
          }
        : { r: 103, g: 80, b: 164 }; // 默认紫色
    };

    // 混合两个颜色
    const mixColors = (color1, color2, weight) => {
      return {
        r: Math.round(color1.r * weight + color2.r * (1 - weight)),
        g: Math.round(color1.g * weight + color2.g * (1 - weight)),
        b: Math.round(color1.b * weight + color2.b * (1 - weight)),
      };
    };

    // RGB 转 Hex
    const rgbToHex = (rgb) => {
      return (
        "#" +
        [rgb.r, rgb.g, rgb.b]
          .map((x) => x.toString(16).padStart(2, "0"))
          .join("")
      );
    };

    const primary = hexToRgb(primaryColor);

    // 设置 primary 相关变量（用户选择的主题色）
    html.style.setProperty("--monet-primary", primaryColor);
    html.style.setProperty("--monet-primary-container", primaryColor + "30");
    html.style.setProperty("--monet-on-primary", "#ffffff");
    html.style.setProperty("--monet-on-primary-container", primaryColor);
    html.style.setProperty("--monet-secondary-container", primaryColor + "30");
    html.style.setProperty("--monet-on-secondary-container", primaryColor);

    // 根据主题色生成 surface 颜色（Material 3 风格）
    if (isDark) {
      // 深色模式：使用深灰色基底，轻微混入主题色
      const darkBase = { r: 20, g: 18, b: 24 }; // #141218
      const darkMid = { r: 33, g: 31, b: 38 }; // #211f26
      const darkHigh = { r: 43, g: 41, b: 48 }; // #2b2930
      const darkHighest = { r: 54, g: 52, b: 59 }; // #36343b

      // 混入约 5% 的主题色，使 surface 带有主题色调
      const surface = mixColors(primary, darkBase, 0.05);
      const surfaceContainer = mixColors(primary, darkMid, 0.05);
      const surfaceContainerLow = mixColors(
        primary,
        { r: 29, g: 27, b: 32 },
        0.05,
      );
      const surfaceContainerHigh = mixColors(primary, darkHigh, 0.05);
      const surfaceContainerHighest = mixColors(primary, darkHighest, 0.05);

      html.style.setProperty("--monet-surface", rgbToHex(surface));
      html.style.setProperty("--monet-on-surface", "#e6e0e9");
      html.style.setProperty("--monet-surface-variant", "#49454f");
      html.style.setProperty("--monet-on-surface-variant", "#cac4d0");
      html.style.setProperty(
        "--monet-surface-container",
        rgbToHex(surfaceContainer),
      );
      html.style.setProperty(
        "--monet-surface-container-low",
        rgbToHex(surfaceContainerLow),
      );
      html.style.setProperty(
        "--monet-surface-container-high",
        rgbToHex(surfaceContainerHigh),
      );
      html.style.setProperty(
        "--monet-surface-container-highest",
        rgbToHex(surfaceContainerHighest),
      );
      html.style.setProperty("--monet-background", rgbToHex(surface));
      html.style.setProperty("--monet-on-background", "#e6e0e9");
      html.style.setProperty("--monet-outline", "#938f99");
      html.style.setProperty("--monet-outline-variant", "#49454f");
      html.style.setProperty("--monet-secondary", "#ccc2dc");
      html.style.setProperty("--monet-on-secondary", "#332d41");
    } else {
      // 浅色模式：使用白色基底，轻微混入主题色
      const white = { r: 255, g: 255, b: 255 };
      const lightBase = { r: 254, g: 247, b: 255 }; // 非常浅的基底

      // 混入约 3-8% 的主题色，使 surface 带有主题色调
      const surface = mixColors(primary, white, 0.03);
      const surfaceContainerLowest = mixColors(primary, white, 0.02);
      const surfaceContainerLow = mixColors(primary, white, 0.04);
      const surfaceContainer = mixColors(primary, white, 0.06);
      const surfaceContainerHigh = mixColors(primary, white, 0.08);
      const surfaceContainerHighest = mixColors(primary, white, 0.1);

      html.style.setProperty("--monet-surface", rgbToHex(surface));
      html.style.setProperty("--monet-on-surface", "#1d1b20");
      html.style.setProperty(
        "--monet-surface-variant",
        rgbToHex(mixColors(primary, { r: 231, g: 224, b: 236 }, 0.15)),
      );
      html.style.setProperty("--monet-on-surface-variant", "#49454f");
      html.style.setProperty(
        "--monet-surface-container",
        rgbToHex(surfaceContainer),
      );
      html.style.setProperty(
        "--monet-surface-container-low",
        rgbToHex(surfaceContainerLow),
      );
      html.style.setProperty(
        "--monet-surface-container-high",
        rgbToHex(surfaceContainerHigh),
      );
      html.style.setProperty(
        "--monet-surface-container-highest",
        rgbToHex(surfaceContainerHighest),
      );
      html.style.setProperty("--monet-background", rgbToHex(surface));
      html.style.setProperty("--monet-on-background", "#1d1b20");
      html.style.setProperty("--monet-outline", "#79747e");
      html.style.setProperty(
        "--monet-outline-variant",
        rgbToHex(mixColors(primary, { r: 202, g: 196, b: 208 }, 0.1)),
      );
      html.style.setProperty(
        "--monet-secondary",
        rgbToHex(mixColors(primary, { r: 98, g: 91, b: 113 }, 0.2)),
      );
      html.style.setProperty("--monet-on-secondary", "#ffffff");
    }
  }

  /**
   * 移除所有内联设置的 Monet 变量
   */
  removeAllMonetVariables() {
    const html = document.documentElement;
    html.style.removeProperty("--monet-primary");
    html.style.removeProperty("--monet-primary-container");
    html.style.removeProperty("--monet-on-primary");
    html.style.removeProperty("--monet-on-primary-container");
    html.style.removeProperty("--monet-secondary");
    html.style.removeProperty("--monet-on-secondary");
    html.style.removeProperty("--monet-secondary-container");
    html.style.removeProperty("--monet-on-secondary-container");
    html.style.removeProperty("--monet-surface");
    html.style.removeProperty("--monet-on-surface");
    html.style.removeProperty("--monet-surface-variant");
    html.style.removeProperty("--monet-on-surface-variant");
    html.style.removeProperty("--monet-surface-container");
    html.style.removeProperty("--monet-surface-container-low");
    html.style.removeProperty("--monet-surface-container-high");
    html.style.removeProperty("--monet-surface-container-highest");
    html.style.removeProperty("--monet-background");
    html.style.removeProperty("--monet-on-background");
    html.style.removeProperty("--monet-outline");
    html.style.removeProperty("--monet-outline-variant");
  }

  applyStoredTheme(): void {
    // 应用存储的主题模式
    const savedTheme = localStorage.getItem("theme") || "auto";
    setTheme(savedTheme as "light" | "dark" | "auto");

    // 应用存储的主题色
    const savedColor = localStorage.getItem("themeColor") || "#6750A4";
    setColorScheme(savedColor);

    // 应用莫奈取色设置
    const savedMonet = localStorage.getItem("monetEnabled");
    const isDark = window.matchMedia("(prefers-color-scheme: dark)").matches;
    const html = document.documentElement;

    if (savedMonet !== "false" && savedTheme === "auto") {
      // 自动模式 + 莫奈取色开启：使用 KernelSU 注入的变量
      html.classList.add("mdui-theme-auto");
      html.classList.remove("mdui-theme-light", "mdui-theme-dark");
      this.removeAllMonetVariables();
    } else if (savedTheme === "auto") {
      // 自动模式 + 莫奈取色关闭：使用用户选择的主题色
      html.classList.remove("mdui-theme-auto");
      html.classList.add(isDark ? "mdui-theme-dark" : "mdui-theme-light");
      this.applyAllMonetVariables(savedColor, isDark);
    } else {
      // 浅色或深色模式：使用用户选择的主题色
      const forceDark = savedTheme === "dark";
      this.applyAllMonetVariables(savedColor, forceDark);
    }
  }

  setupMonetToggle(): void {
    const monetToggle = document.getElementById(
      "monet-toggle",
    ) as HTMLInputElement | null;
    if (!monetToggle) return;

    monetToggle.addEventListener("change", (e) => {
      const enabled = (e.target as HTMLInputElement).checked;
      localStorage.setItem("monetEnabled", String(enabled));
      this.applyMonetSetting(enabled);
      toast(
        I18nService.t("settings.monet.toast_toggled") +
          (enabled
            ? I18nService.t("common.enabled")
            : I18nService.t("common.disabled")),
      );
    });
  }

  updateMonetToggleState(): void {
    const savedTheme = localStorage.getItem("theme") || "auto";
    const monetToggle = document.getElementById(
      "monet-toggle",
    ) as HTMLInputElement | null;
    const savedMonet = localStorage.getItem("monetEnabled");

    if (monetToggle) {
      if (savedTheme === "auto") {
        monetToggle.disabled = false;
        if (savedMonet !== null) {
          monetToggle.checked = savedMonet === "true";
        } else {
          monetToggle.checked = true;
        }
        // 注意：不在这里调用 applyMonetSetting()
        // 该方法只用于更新 UI 状态，实际的主题应用在初始化时已完成
      } else {
        monetToggle.disabled = true;
        monetToggle.checked = false;
      }
    }
  }

  applyMonetSetting(enabled) {
    const html = document.documentElement;
    const savedColor = localStorage.getItem("themeColor") || "#6750A4";
    const isDark = window.matchMedia("(prefers-color-scheme: dark)").matches;

    if (enabled) {
      html.classList.add("mdui-theme-auto");
      html.classList.remove("mdui-theme-light", "mdui-theme-dark");
      // 移除所有内联设置的 monet 变量，让 CSS 使用 monet.css 中定义的规则
      this.removeAllMonetVariables();
      setColorScheme(savedColor);
    } else {
      html.classList.remove("mdui-theme-auto");
      html.classList.add(isDark ? "mdui-theme-dark" : "mdui-theme-light");
      // 设置所有 monet 变量，使用 MDUI 生成的值覆盖 monet.css 的默认值
      this.applyAllMonetVariables(savedColor, isDark);
    }

    this.ui.statusPage.updateSpeedChartColors();
  }

  showAboutDialog() {
    const dialog = document.createElement("mdui-dialog") as any;
    dialog.id = "about-dialog";
    dialog.headline = I18nService.t("settings.about.title");
    dialog.innerHTML = `
            <div style="text-align: center; padding: 16px 0;">
                <img src="${logoUrl}" alt="NetProxy" style="width: 72px; height: 72px; border-radius: 16px;">
                <h2 style="margin: 16px 0 8px;">NetProxy</h2>
                <p style="color: var(--mdui-color-on-surface-variant); margin: 0;">${I18nService.t("settings.about.description")}</p>
                <p style="margin-top: 16px;">
                    <mdui-chip icon="code">Xray Core</mdui-chip>
                    <mdui-chip icon="android">Magisk / KernelSU</mdui-chip>
                </p>
            </div>
            <mdui-divider></mdui-divider>
            <mdui-list>
                <mdui-list-item id="about-github">
                    <mdui-icon slot="icon" class="mdui-color-on-surface-variant" style="width: 24px; height: 24px;">
                        <svg viewBox="0 0 24 24" width="24" height="24" fill="currentColor">
                            <path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z"/>
                        </svg>
                    </mdui-icon>
                    GitHub
                </mdui-list-item>
                <mdui-list-item id="about-telegram">
                    <mdui-icon slot="icon" class="mdui-color-on-surface-variant" style="width: 24px; height: 24px;">
                        <svg viewBox="0 0 24 24" width="24" height="24" fill="currentColor">
                            <path d="M12 0C5.373 0 0 5.373 0 12s5.373 12 12 12 12-5.373 12-12S18.627 0 12 0zm5.562 8.161c-.18 1.897-.962 6.502-1.359 8.627-.168.9-.5 1.201-.82 1.23-.696.064-1.225-.46-1.901-.903-1.056-.693-1.653-1.124-2.678-1.8-1.185-.78-.417-1.21.258-1.91.177-.184 3.247-2.977 3.307-3.23.007-.032.014-.15-.056-.212s-.174-.041-.249-.024c-.106.024-1.793 1.14-5.061 3.345-.48.33-.913.49-1.302.48-.428-.008-1.252-.241-1.865-.44-.752-.245-1.349-.374-1.297-.789.027-.216.325-.437.893-.663 3.498-1.524 5.83-2.529 6.998-3.014 3.332-1.386 4.025-1.627 4.477-1.635.099-.002.321.023.465.141.121.1.154.234.17.331.015.099.034.323.019.498z"/>
                        </svg>
                    </mdui-icon>
                    ${I18nService.t("settings.about.group")}
                </mdui-list-item>
            </mdui-list>
            <mdui-button slot="action" variant="text">${I18nService.t("settings.about.close")}</mdui-button>
        `;

    document.body.appendChild(dialog);
    requestAnimationFrame(() => {
      dialog.open = true;
    });

    dialog.querySelector("#about-github")?.addEventListener("click", () => {
      SettingsService.openExternalUrl(
        "https://github.com/Fanju6/NetProxy-Magisk",
      );
    });

    dialog.querySelector("#about-telegram")?.addEventListener("click", () => {
      SettingsService.openExternalUrl("https://t.me/NetProxy_Magisk");
    });

    dialog.querySelector("mdui-button").addEventListener("click", () => {
      dialog.open = false;
      setTimeout(() => dialog.remove(), 300);
    });

    dialog.addEventListener("closed", () => {
      setTimeout(() => dialog.remove(), 300);
    });
  }

  // 加载模块设置
  async loadModuleSettings() {
    try {
      const settings = await SettingsService.getModuleSettings();

      const autoStartSwitch = document.getElementById(
        "module-auto-start",
      ) as HTMLInputElement | null;
      if (autoStartSwitch) {
        autoStartSwitch.checked = settings.auto_start;
      }

      const oneplusFixSwitch = document.getElementById(
        "module-oneplus-fix",
      ) as HTMLInputElement | null;
      if (oneplusFixSwitch) {
        oneplusFixSwitch.checked = settings.oneplus_a16_fix;
      }
    } catch (error) {
      console.error("Failed to load module settings:", error);
    }
  }

  // ===================== 语言设置页面 =====================

  setupLanguagePage(): void {
    // 返回按钮
    const backBtn = document.getElementById("language-back-btn");
    if (backBtn) {
      backBtn.addEventListener("click", () => {
        this.ui.switchPage("settings");
      });
    }

    // 语言切换
    const languageGroup = document.getElementById("language-group") as any;
    if (languageGroup) {
      languageGroup.addEventListener("change", (e: Event) => {
        const lang = (e.target as HTMLInputElement).value;
        I18nService.setLanguage(lang);
        // 刷新当前页面状态
        this.loadLanguageSettings();
      });
    }
  }

  loadLanguageSettings(): void {
    const languageGroup = document.getElementById("language-group") as any;
    if (languageGroup) {
      languageGroup.value = I18nService.getLanguage();
    }
  }

  // ===================== 日志页面管理 =====================

  setupLogsPage(): void {
    this.setupLogsTabs();
    this.setupLogsAutoRefresh();
    this.setupLogsActions();
  }

  setupLogsTabs(): void {
    const tabsEl = document.getElementById("logs-tabs");
    if (!tabsEl) return;

    // 注入滚动样式到 Shadow DOM
    requestAnimationFrame(() => {
      const shadowRoot = tabsEl.shadowRoot;
      if (shadowRoot) {
        const container = shadowRoot.querySelector(
          '[part="container"]',
        ) as HTMLElement;
        if (container) {
          container.style.cssText =
            "display: flex; flex-wrap: nowrap; overflow-x: auto; -webkit-overflow-scrolling: touch;";
        }

        // 让每个 tab 保持自然宽度不收缩
        const slots = shadowRoot.querySelectorAll("slot");
        slots.forEach((slot) => {
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
        (tab as HTMLElement).style.cssText =
          "flex-shrink: 0; white-space: nowrap;";
      });
    });

    // 绑定 tab 切换事件
    tabsEl.addEventListener("change", (e: any) => {
      this._logsSelectedTab = e.target.value;
      this.loadActiveLog();
    });
  }

  setupLogsAutoRefresh(): void {
    const toggle = document.getElementById("logs-auto-refresh") as any;
    if (!toggle) return;

    toggle.addEventListener("change", () => {
      this._logsAutoRefreshEnabled = toggle.checked;
      if (this._logsAutoRefreshEnabled) {
        this.startLogAutoRefresh();
      } else {
        this.stopLogAutoRefresh();
      }
    });
  }

  setupLogsActions(): void {
    document
      .getElementById("export-logs-btn")
      ?.addEventListener("click", () => this.exportLogs());
    document
      .getElementById("export-all-btn")
      ?.addEventListener("click", () => this.exportAll());
    // document.getElementById('clear-logs-btn')?.addEventListener('click', () => this.clearDebugLogs());
  }

  startLogAutoRefresh(): void {
    this.stopLogAutoRefresh(); // 先停止已有的
    this._logsAutoRefreshInterval = setInterval(() => {
      this.loadActiveLog();
    }, this._logsAutoRefreshMs);
  }

  stopLogAutoRefresh(): void {
    if (this._logsAutoRefreshInterval) {
      clearInterval(this._logsAutoRefreshInterval);
      this._logsAutoRefreshInterval = null;
    }
  }

  // 根据当前选中的 tab 加载日志
  loadActiveLog(): void {
    switch (this._logsSelectedTab) {
      case "service":
        this.loadServiceLog();
        break;
      case "xray":
        this.loadXrayLog();
        break;
    }
  }

  async updateLogs(): Promise<void> {
    await this.loadActiveLog();
  }

  async loadServiceLog(): Promise<void> {
    const container = document.getElementById("service-log");
    if (!container) return;

    try {
      const log = await SettingsService.getServiceLog();
      this.renderLog(container, log);
    } catch (error: any) {
      container.innerHTML = `<span style="color: var(--mdui-color-error);">${I18nService.t("logs.load_failed")}: ${error.message}</span>`;
    }
  }

  async loadXrayLog(): Promise<void> {
    const container = document.getElementById("xray-log");
    if (!container) return;

    try {
      const log = await SettingsService.getXrayLog();
      this.renderLog(container, log);
    } catch (error: any) {
      container.innerHTML = `<span style="color: var(--mdui-color-error);">${I18nService.t("logs.load_failed")}: ${error.message}</span>`;
    }
  }

  renderLog(container: HTMLElement, log: string): void {
    if (!log || log.trim() === "") {
      container.innerHTML =
        '<span style="color: var(--mdui-color-on-surface-variant); font-style: italic;">No logs available</span>';
      return;
    }

    // 将日志文本转为 HTML，保留换行
    const escapedLog = log
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/\n/g, "<br>");

    container.innerHTML = `<pre style="margin: 0; white-space: pre-wrap; word-break: break-all; font-size: 12px; line-height: 1.6;">${escapedLog}</pre>`;

    // 滚动到底部
    container.scrollTop = container.scrollHeight;
  }

  // 导出日志
  async exportLogs(): Promise<void> {
    try {
      const result: any = await SettingsService.exportLogs();
      if (result.success) {
        toast(I18nService.t("logs.saved_to") + result.path);
      } else {
        toast(I18nService.t("logs.save_failed"));
      }
    } catch (error: any) {
      toast(I18nService.t("logs.save_failed") + ": " + error.message);
    }
  }

  // 导出日志和配置
  async exportAll(): Promise<void> {
    try {
      const result: any = await SettingsService.exportAll();
      if (result.success) {
        toast(I18nService.t("logs.saved_all_to") + result.path);
      } else {
        toast(I18nService.t("logs.save_failed"));
      }
    } catch (error: any) {
      toast(I18nService.t("logs.save_failed") + ": " + error.message);
    }
  }

  // 清空调试日志
  async clearDebugLogs(): Promise<void> {
    try {
      await (SettingsService as any).clearDebugLogs();
      toast(I18nService.t("logs.debug_cleared"));
      this.loadActiveLog();
    } catch (error: any) {
      toast(I18nService.t("logs.unknown_error") + ": " + error.message);
    }
  }

  // 页面离开时停止自动刷新
  onLogsPageLeave(): void {
    this.stopLogAutoRefresh();
    const toggle = document.getElementById("logs-auto-refresh") as any;
    if (toggle) {
      toggle.checked = false;
    }
    this._logsAutoRefreshEnabled = false;
  }
}
