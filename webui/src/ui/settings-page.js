import { toast } from '../utils/toast.js';
import { KSUService } from '../services/ksu-service.js';
import { setColorScheme } from 'mdui/functions/setColorScheme.js';
import { setTheme } from 'mdui/functions/setTheme.js';
const logoUrl = 'https://ghfast.top/https://raw.githubusercontent.com/Fanju6/NetProxy-Magisk/refs/heads/main/logo.png';

export class SettingsPageManager {
    constructor(ui) {
        this.ui = ui;
        this.routingRules = [];
        this.editingRuleIndex = -1;
        this.proxyKeys = [
            'proxy_mobile', 'proxy_wifi', 'proxy_hotspot', 'proxy_usb',
            'proxy_tcp', 'proxy_udp', 'proxy_ipv6'
        ];
        this.setupEventListeners();
        this.setupRoutingRulesPage();
        this.setupProxySettingsPage();
        this.setupThemePage();
        this.applyStoredTheme();
    }

    setupEventListeners() {
        // 日志入口
        const logsEntry = document.getElementById('settings-logs-entry');
        if (logsEntry) {
            logsEntry.addEventListener('click', () => {
                this.ui.switchPage('logs');
            });
        }

        // 日志页返回按钮
        const logsBackBtn = document.getElementById('logs-back-btn');
        if (logsBackBtn) {
            logsBackBtn.addEventListener('click', () => {
                this.ui.switchPage('settings');
            });
        }

        // 路由设置入口
        const routingEntry = document.getElementById('settings-routing-entry');
        if (routingEntry) {
            routingEntry.addEventListener('click', () => {
                this.ui.switchPage('routing');
                this.loadRoutingRules();
            });
        }

        // 代理设置入口
        const proxyEntry = document.getElementById('settings-proxy-entry');
        if (proxyEntry) {
            proxyEntry.addEventListener('click', () => {
                this.ui.switchPage('proxy-settings');
                this.loadProxySettings();
            });
        }

        // 模块设置入口
        const moduleEntry = document.getElementById('settings-module-entry');
        if (moduleEntry) {
            moduleEntry.addEventListener('click', () => {
                this.ui.switchPage('module');
                this.loadModuleSettings();
            });
        }

        // 模块设置页返回按钮
        const moduleBackBtn = document.getElementById('module-back-btn');
        if (moduleBackBtn) {
            moduleBackBtn.addEventListener('click', () => {
                this.ui.switchPage('settings');
            });
        }

        // 模块设置开关
        const autoStartSwitch = document.getElementById('module-auto-start');
        if (autoStartSwitch) {
            autoStartSwitch.addEventListener('change', async (e) => {
                try {
                    await KSUService.setModuleSetting('AUTO_START', e.target.checked);
                    toast(`开机自启已${e.target.checked ? '启用' : '禁用'}`);
                } catch (error) {
                    toast('设置失败: ' + error.message, true);
                    e.target.checked = !e.target.checked;
                }
            });
        }

        const oneplusFixSwitch = document.getElementById('module-oneplus-fix');
        if (oneplusFixSwitch) {
            oneplusFixSwitch.addEventListener('change', async (e) => {
                try {
                    await KSUService.setModuleSetting('ONEPLUS_A16_FIX', e.target.checked);
                    toast(`OnePlus A16 兼容性修复已${e.target.checked ? '启用' : '禁用'}`);
                } catch (error) {
                    toast('设置失败: ' + error.message, true);
                    e.target.checked = !e.target.checked;
                }
            });
        }

        // 主题设置入口
        const themeEntry = document.getElementById('settings-theme');
        if (themeEntry) {
            themeEntry.addEventListener('click', () => {
                this.ui.switchPage('theme');
                this.loadThemeSettings();
            });
        }

        // 关于
        const aboutEntry = document.getElementById('settings-about');
        if (aboutEntry) {
            aboutEntry.addEventListener('click', () => {
                this.showAboutDialog();
            });
        }
    }

    // ===================== 路由规则管理 =====================

    setupRoutingRulesPage() {
        // 返回按钮
        const backBtn = document.getElementById('routing-back-btn');
        if (backBtn) {
            backBtn.addEventListener('click', () => {
                this.ui.switchPage('settings');
            });
        }

        // 添加规则按钮
        const addBtn = document.getElementById('add-routing-rule-btn');
        if (addBtn) {
            addBtn.addEventListener('click', () => {
                this.showRuleDialog();
            });
        }

        // 规则对话框事件
        const cancelBtn = document.getElementById('rule-cancel');
        if (cancelBtn) {
            cancelBtn.addEventListener('click', () => {
                document.getElementById('routing-rule-dialog').open = false;
            });
        }

        const saveBtn = document.getElementById('rule-save');
        if (saveBtn) {
            saveBtn.addEventListener('click', () => {
                this.saveRule();
            });
        }
    }

    async loadRoutingRules() {
        try {
            this.routingRules = await KSUService.getRoutingRules();
            this.renderRoutingRules();
        } catch (error) {
            console.error('加载路由规则失败:', error);
            toast('加载路由规则失败');
        }
    }

    renderRoutingRules() {
        const listEl = document.getElementById('routing-rules-list');
        if (!listEl) return;

        listEl.innerHTML = '';

        if (this.routingRules.length === 0) {
            listEl.innerHTML = `
                <mdui-list-item>
                    <span slot="description">暂无规则，点击右上角添加</span>
                </mdui-list-item>
            `;
            return;
        }

        this.routingRules.forEach((rule, index) => {
            const item = document.createElement('mdui-list-item');

            // 构建描述
            const parts = [];
            if (rule.domain) parts.push(`域名: ${rule.domain}`);
            if (rule.ip) parts.push(`IP: ${rule.ip}`);
            if (rule.port) parts.push(`端口: ${rule.port}`);
            if (rule.network) parts.push(`网络: ${rule.network}`);
            if (rule.protocol) parts.push(`协议: ${rule.protocol}`);

            const description = parts.length > 0 ? parts.join(' | ') : '无条件';
            const outboundLabel = { proxy: '代理', direct: '直连', block: '阻断' }[rule.outboundTag] || rule.outboundTag;

            item.setAttribute('headline', rule.name || `规则 ${index + 1}`);

            // 使用 description slot 显示详情和出站
            const descDiv = document.createElement('div');
            descDiv.slot = 'description';
            descDiv.style.cssText = 'display: flex; justify-content: space-between; width: 100%;';

            const descSpan = document.createElement('span');
            descSpan.textContent = description;
            descSpan.style.cssText = 'overflow: hidden; text-overflow: ellipsis; white-space: nowrap; flex: 1;';

            const outboundSpan = document.createElement('span');
            outboundSpan.textContent = outboundLabel;
            outboundSpan.style.cssText = 'margin-left: 8px; padding: 2px 6px; border-radius: 4px; font-size: 11px; background: var(--mdui-color-secondary-container); color: var(--mdui-color-on-secondary-container);';

            descDiv.appendChild(descSpan);
            descDiv.appendChild(outboundSpan);
            item.appendChild(descDiv);

            // 右侧容器：开关和菜单
            const endContainer = document.createElement('div');
            endContainer.slot = 'end-icon';
            endContainer.style.cssText = 'display: flex; align-items: center; gap: 4px;';

            // 启用开关
            const switchEl = document.createElement('mdui-switch');
            switchEl.checked = rule.enabled !== false;
            switchEl.addEventListener('change', async (e) => {
                e.stopPropagation();
                rule.enabled = e.target.checked;
                await this.saveRulesToBackend();
            });
            endContainer.appendChild(switchEl);

            // 菜单
            const dropdown = document.createElement('mdui-dropdown');
            dropdown.setAttribute('placement', 'bottom-end');

            const menuBtn = document.createElement('mdui-button-icon');
            menuBtn.setAttribute('slot', 'trigger');
            menuBtn.setAttribute('icon', 'more_vert');
            menuBtn.addEventListener('click', (e) => e.stopPropagation());
            dropdown.appendChild(menuBtn);

            const menu = document.createElement('mdui-menu');

            // 编辑
            const editItem = document.createElement('mdui-menu-item');
            editItem.innerHTML = '<mdui-icon slot="icon" name="edit"></mdui-icon>编辑';
            editItem.addEventListener('click', (e) => {
                e.stopPropagation();
                dropdown.open = false;
                this.showRuleDialog(rule, index);
            });
            menu.appendChild(editItem);

            // 删除
            const deleteItem = document.createElement('mdui-menu-item');
            deleteItem.innerHTML = '<mdui-icon slot="icon" name="delete"></mdui-icon>删除';
            deleteItem.style.color = 'var(--mdui-color-error)';
            deleteItem.addEventListener('click', async (e) => {
                e.stopPropagation();
                dropdown.open = false;
                if (await this.ui.confirm(`确定要删除规则 "${rule.name || `规则 ${index + 1}`}" 吗？`)) {
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

    showRuleDialog(rule = null, index = -1) {
        this.editingRuleIndex = index;
        const dialog = document.getElementById('routing-rule-dialog');

        // 设置标题
        dialog.headline = rule ? '编辑规则' : '添加规则';

        // 填充表单
        document.getElementById('rule-name').value = rule?.name || '';
        document.getElementById('rule-domain').value = rule?.domain || '';
        document.getElementById('rule-ip').value = rule?.ip || '';
        document.getElementById('rule-port').value = rule?.port || '';
        document.getElementById('rule-protocol').value = rule?.protocol || '';
        document.getElementById('rule-network').value = rule?.network || '';
        document.getElementById('rule-outbound').value = rule?.outboundTag || 'proxy';

        dialog.open = true;
    }

    async saveRule() {
        const name = document.getElementById('rule-name').value.trim();
        const domain = document.getElementById('rule-domain').value.trim();
        const ip = document.getElementById('rule-ip').value.trim();
        const port = document.getElementById('rule-port').value.trim();
        const protocol = document.getElementById('rule-protocol').value.trim();
        const network = document.getElementById('rule-network').value.trim();
        const outboundTag = document.getElementById('rule-outbound').value;

        // 验证
        if (!domain && !ip && !port && !protocol && !network) {
            toast('请至少填写一个匹配条件');
            return;
        }

        const rule = {
            name: name || (this.editingRuleIndex >= 0 ? `规则 ${this.editingRuleIndex + 1}` : `规则 ${this.routingRules.length + 1}`),
            type: 'field',
            domain,
            ip,
            port,
            protocol,
            network,
            outboundTag,
            enabled: true
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
        document.getElementById('routing-rule-dialog').open = false;
        toast(this.editingRuleIndex >= 0 ? '规则已更新' : '规则已添加');
    }

    async saveRulesToBackend() {
        try {
            await KSUService.saveRoutingRules(this.routingRules);
            await KSUService.applyRoutingRules(this.routingRules);
        } catch (error) {
            console.error('保存规则失败:', error);
            toast('保存失败: ' + error.message);
        }
    }

    // ===================== 代理设置页面 =====================

    setupProxySettingsPage() {
        // 返回按钮
        const backBtn = document.getElementById('proxy-settings-back-btn');
        if (backBtn) {
            backBtn.addEventListener('click', () => {
                this.ui.switchPage('settings');
            });
        }

        // 为每个开关绑定事件
        for (const key of this.proxyKeys) {
            // key 格式: proxy_mobile -> HTML id: proxy-mobile
            const htmlId = key.replace('_', '-');
            const switchEl = document.getElementById(htmlId);
            if (switchEl) {
                switchEl.addEventListener('change', async (e) => {
                    const value = e.target.checked;
                    await this.setProxySetting(key, value);
                });
            }
        }

        // 代理模式选择器
        const proxyModeGroup = document.getElementById('proxy-mode-settings');
        if (proxyModeGroup) {
            proxyModeGroup.addEventListener('change', async (e) => {
                const value = e.target.value;
                await this.setProxyMode(value);
            });
        }
    }

    async loadProxySettings() {
        try {
            const settings = await KSUService.getProxySettings();
            for (const key of this.proxyKeys) {
                const htmlId = key.replace('_', '-');
                const switchEl = document.getElementById(htmlId);
                if (switchEl) {
                    switchEl.checked = settings[key] === true;
                }
            }

            // 加载代理模式
            const proxyMode = await KSUService.getProxyMode();
            const proxyModeGroup = document.getElementById('proxy-mode-settings');
            const proxyModeDesc = document.getElementById('proxy-mode-desc-settings');
            if (proxyModeGroup) {
                proxyModeGroup.value = String(proxyMode);
            }
            if (proxyModeDesc) {
                this.updateProxyModeDesc(proxyMode);
            }
        } catch (error) {
            console.error('加载代理设置失败:', error);
        }
    }

    async setProxySetting(key, value) {
        try {
            await KSUService.setProxySetting(key, value);
            toast(`已${value ? '启用' : '禁用'}`);
        } catch (error) {
            toast('设置失败: ' + error.message);
            // 恢复开关状态
            const htmlId = key.replace('_', '-');
            const switchEl = document.getElementById(htmlId);
            if (switchEl) {
                switchEl.checked = !value;
            }
        }
    }

    async setProxyMode(value) {
        try {
            await KSUService.setProxyMode(value);
            this.updateProxyModeDesc(value);
            const modeNames = { '0': '自动', '1': 'TPROXY', '2': 'REDIRECT' };
            toast(`代理模式已设为: ${modeNames[value] || value}`);
        } catch (error) {
            toast('设置代理模式失败: ' + error.message);
        }
    }

    updateProxyModeDesc(mode) {
        const desc = document.getElementById('proxy-mode-desc-settings');
        if (!desc) return;
        const descs = {
            '0': '自动检测最佳代理方式',
            '1': '强制使用 TPROXY 透明代理',
            '2': '强制使用 REDIRECT 重定向'
        };
        desc.textContent = descs[String(mode)] || descs['0'];
    }

    // ===================== 主题页面 =====================

    setupThemePage() {
        // 返回按钮
        const backBtn = document.getElementById('theme-back-btn');
        if (backBtn) {
            backBtn.addEventListener('click', () => {
                this.ui.switchPage('settings');
            });
        }

        // 模式选择
        const modeGroup = document.getElementById('theme-mode-group');
        if (modeGroup) {
            modeGroup.addEventListener('change', (e) => {
                const mode = e.target.value;
                this.applyThemeMode(mode);
                this.updateMonetSwitchState(mode);
            });
        }

        // 莫奈取色开关
        const monetSwitch = document.getElementById('monet-switch');
        if (monetSwitch) {
            monetSwitch.addEventListener('change', (e) => {
                const enabled = e.target.checked;
                this.applyMonetSetting(enabled);
            });
        }

        // 颜色选择
        const colorPalette = document.getElementById('color-palette');
        if (colorPalette) {
            colorPalette.addEventListener('click', (e) => {
                const colorItem = e.target.closest('.color-item');
                if (colorItem) {
                    const color = colorItem.dataset.color;
                    this.applyThemeColor(color);
                    this.updateColorSelection(color);
                }
            });
        }
    }

    loadThemeSettings() {
        const savedTheme = localStorage.getItem('theme') || 'auto';
        const savedColor = localStorage.getItem('themeColor') || '#6750A4';
        const monetEnabled = localStorage.getItem('monetEnabled') === 'true';

        // 设置模式选择
        const modeGroup = document.getElementById('theme-mode-group');
        if (modeGroup) {
            modeGroup.value = savedTheme;
        }

        // 设置莫奈取色开关状态
        const monetSwitch = document.getElementById('monet-switch');
        if (monetSwitch) {
            monetSwitch.checked = monetEnabled;
        }

        // 更新莫奈开关的可用状态
        this.updateMonetSwitchState(savedTheme);

        // 设置颜色选择
        this.updateColorSelection(savedColor);

        // 更新颜色选择面板的可见性
        this.updateColorPaletteVisibility();
    }

    updateColorSelection(selectedColor) {
        const colorItems = document.querySelectorAll('.color-item');
        colorItems.forEach(item => {
            if (item.dataset.color === selectedColor) {
                item.classList.add('selected');
            } else {
                item.classList.remove('selected');
            }
        });
    }

    updateMonetSwitchState(mode) {
        const monetSwitch = document.getElementById('monet-switch');
        const monetDescription = document.getElementById('monet-description');

        if (monetSwitch) {
            // 莫奈取色仅在自动模式下可用
            const isAutoMode = mode === 'auto';
            monetSwitch.disabled = !isAutoMode;

            if (monetDescription) {
                if (isAutoMode) {
                    monetDescription.textContent = '使用系统壁纸颜色作为主题色';
                } else {
                    monetDescription.textContent = '使用系统壁纸颜色作为主题色（仅自动模式可用）';
                    // 非自动模式下，关闭莫奈取色
                    monetSwitch.checked = false;
                    localStorage.setItem('monetEnabled', 'false');
                    // 移除 monet-enabled 类
                    document.documentElement.classList.remove('monet-enabled');
                    // 恢复手动主题色
                    const savedColor = localStorage.getItem('themeColor');
                    if (savedColor) {
                        setColorScheme(savedColor);
                    }
                }
            }
        }

        this.updateColorPaletteVisibility();
    }

    updateColorPaletteVisibility() {
        const colorPaletteCard = document.getElementById('color-palette-card');
        const monetSwitch = document.getElementById('monet-switch');
        const savedTheme = localStorage.getItem('theme') || 'auto';

        if (colorPaletteCard && monetSwitch) {
            // 当莫奈取色启用且处于自动模式时，隐藏颜色选择面板
            const shouldHide = monetSwitch.checked && savedTheme === 'auto';
            colorPaletteCard.style.display = shouldHide ? 'none' : 'block';
        }
    }

    applyMonetSetting(enabled) {
        localStorage.setItem('monetEnabled', enabled.toString());

        if (enabled) {
            // 启用莫奈取色：添加 monet-enabled 类，让 monet.css 使用 KernelSU 变量
            document.documentElement.classList.add('monet-enabled');
            toast('已启用莫奈取色');
        } else {
            // 禁用莫奈取色：移除 monet-enabled 类
            document.documentElement.classList.remove('monet-enabled');
            // 恢复手动设置的主题色
            const savedColor = localStorage.getItem('themeColor');
            if (savedColor) {
                setColorScheme(savedColor);
            }
            toast('已禁用莫奈取色');
        }

        this.updateColorPaletteVisibility();
    }

    applyThemeMode(mode) {
        localStorage.setItem('theme', mode);
        setTheme(mode);
        toast(`已切换到${mode === 'auto' ? '自动' : mode === 'light' ? '浅色' : '深色'}模式`);
    }

    applyThemeColor(color) {
        localStorage.setItem('themeColor', color);
        // 用户手动选择颜色时，需要关闭莫奈取色以使手动颜色生效
        const monetEnabled = localStorage.getItem('monetEnabled') === 'true';
        const savedTheme = localStorage.getItem('theme') || 'auto';
        if (monetEnabled && savedTheme === 'auto') {
            // 禁用莫奈取色，因为用户手动选择了颜色
            localStorage.setItem('monetEnabled', 'false');
            // 移除 monet-enabled 类
            document.documentElement.classList.remove('monet-enabled');
            const monetSwitch = document.getElementById('monet-switch');
            if (monetSwitch) {
                monetSwitch.checked = false;
            }
            this.updateColorPaletteVisibility();
        }
        setColorScheme(color);
        toast('主题色已更改');
    }

    applyStoredTheme() {
        // 应用存储的主题模式
        const savedTheme = localStorage.getItem('theme') || 'auto';
        setTheme(savedTheme);

        // 检查是否启用莫奈取色
        const monetEnabled = localStorage.getItem('monetEnabled') === 'true';

        // 只有在自动模式且莫奈取色启用时，才添加 monet-enabled 类
        if (savedTheme === 'auto' && monetEnabled) {
            document.documentElement.classList.add('monet-enabled');
            // 不设置颜色方案，让 monet.css 的变量生效
            return;
        } else {
            // 确保移除 monet-enabled 类
            document.documentElement.classList.remove('monet-enabled');
        }

        // 应用存储的主题色
        const savedColor = localStorage.getItem('themeColor');
        if (savedColor) {
            setColorScheme(savedColor);
        }
    }


    showAboutDialog() {
        const dialog = document.createElement('mdui-dialog');
        dialog.headline = '关于 NetProxy';
        dialog.innerHTML = `
            <div style="text-align: center; padding: 16px 0;">
                <img src="${logoUrl}" alt="NetProxy" style="width: 72px; height: 72px; border-radius: 16px;">
                <h2 style="margin: 16px 0 8px;">NetProxy</h2>
                <p style="color: var(--mdui-color-on-surface-variant); margin: 0;">Android 系统级 Xray 透明代理模块</p>
                <p style="margin-top: 16px;">
                    <mdui-chip icon="code">Xray Core</mdui-chip>
                    <mdui-chip icon="android">Magisk / KernelSU</mdui-chip>
                </p>
            </div>
            <mdui-divider></mdui-divider>
            <mdui-list>
                <mdui-list-item id="about-github">
                    GitHub
                    <mdui-icon slot="end-icon" name="content_copy"></mdui-icon>
                </mdui-list-item>
                <mdui-list-item id="about-telegram">
                    Telegram 群组
                    <mdui-icon slot="end-icon" name="content_copy"></mdui-icon>
                </mdui-list-item>
            </mdui-list>
            <mdui-button slot="action" variant="text">关闭</mdui-button>
        `;

        document.body.appendChild(dialog);
        dialog.open = true;

        dialog.querySelector('#about-github')?.addEventListener('click', () => {
            navigator.clipboard.writeText('https://github.com/Fanju6/NetProxy-Magisk');
            toast('GitHub 链接已复制');
        });

        dialog.querySelector('#about-telegram')?.addEventListener('click', () => {
            navigator.clipboard.writeText('https://t.me/NetProxy_Magisk');
            toast('Telegram 链接已复制');
        });

        dialog.querySelector('mdui-button').addEventListener('click', () => {
            dialog.open = false;
            setTimeout(() => dialog.remove(), 300);
        });

        dialog.addEventListener('closed', () => {
            setTimeout(() => dialog.remove(), 300);
        });
    }

    // 加载模块设置
    async loadModuleSettings() {
        try {
            const settings = await KSUService.getModuleSettings();

            const autoStartSwitch = document.getElementById('module-auto-start');
            if (autoStartSwitch) {
                autoStartSwitch.checked = settings.auto_start;
            }

            const oneplusFixSwitch = document.getElementById('module-oneplus-fix');
            if (oneplusFixSwitch) {
                oneplusFixSwitch.checked = settings.oneplus_a16_fix;
            }
        } catch (error) {
            console.error('Failed to load module settings:', error);
        }
    }

}
