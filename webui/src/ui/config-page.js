import { KSUService } from '../services/ksu-service.js';
import { toast } from '../utils/toast.js';

/**
 * 配置页面管理器 - 支持分组显示
 */
export class ConfigPageManager {
    constructor(ui) {
        this.ui = ui;
        this.expandedGroups = new Set(['默认分组']); // 默认展开的分组
        this.currentOpenDropdown = null; // 当前打开的下拉菜单
    }

    /**
     * 从配置内容解析出站信息
     */
    parseOutboundInfo(content) {
        try {
            const config = JSON.parse(content);
            const outbounds = config.outbounds || [];

            for (const outbound of outbounds) {
                const protocol = outbound.protocol;
                if (!protocol || ['freedom', 'blackhole', 'dns'].includes(protocol)) {
                    continue;
                }

                let address = '';
                let port = '';

                if (outbound.settings) {
                    if (outbound.settings.vnext && outbound.settings.vnext[0]) {
                        address = outbound.settings.vnext[0].address || '';
                        port = outbound.settings.vnext[0].port || '';
                    } else if (outbound.settings.servers && outbound.settings.servers[0]) {
                        address = outbound.settings.servers[0].address || '';
                        port = outbound.settings.servers[0].port || '';
                    }
                }

                return { protocol, address, port };
            }

            return { protocol: 'direct', address: '直连模式', port: '' };
        } catch (e) {
            return { protocol: 'unknown', address: '', port: '' };
        }
    }

    async update() {
        try {
            const listEl = document.getElementById('config-list');

            const groups = await KSUService.getConfigGroups();
            const { config: currentConfig } = await KSUService.getStatus();

            if (groups.length === 0) {
                listEl.innerHTML = '<mdui-list-item><div slot="headline">暂无配置文件</div></mdui-list-item>';
                return;
            }

            // 先构建所有 DOM 元素到一个文档片段中
            const fragment = document.createDocumentFragment();

            for (const group of groups) {
                await this.renderGroup(fragment, group, currentConfig);
            }

            // 所有内容准备好后，一次性替换
            listEl.innerHTML = '';
            listEl.appendChild(fragment);
        } catch (error) {
            console.error('Update config page failed:', error);
        }
    }

    async renderGroup(container, group, currentConfig) {
        const isExpanded = this.expandedGroups.has(group.name);

        // 分组头部
        const header = document.createElement('mdui-list-item');
        header.setAttribute('clickable', '');
        header.style.backgroundColor = 'var(--mdui-color-surface-container)';

        // 展开/收起图标
        const expandIcon = document.createElement('mdui-icon');
        expandIcon.slot = 'icon';
        expandIcon.name = isExpanded ? 'expand_more' : 'chevron_right';
        header.appendChild(expandIcon);

        // 分组标题
        header.setAttribute('headline', `${group.name} (${group.configs.length})`);

        // 订阅分组显示更新时间
        if (group.type === 'subscription' && group.updated) {
            const date = new Date(group.updated);
            header.setAttribute('description', `更新于 ${date.toLocaleDateString()}`);
        }

        // 订阅分组添加刷新和删除按钮
        if (group.type === 'subscription') {
            const refreshBtn = document.createElement('mdui-button-icon');
            refreshBtn.slot = 'end-icon';
            refreshBtn.setAttribute('icon', 'refresh');
            refreshBtn.addEventListener('click', async (e) => {
                e.stopPropagation();
                await this.updateSubscription(group.name);
            });
            header.appendChild(refreshBtn);

            const deleteBtn = document.createElement('mdui-button-icon');
            deleteBtn.slot = 'end-icon';
            deleteBtn.setAttribute('icon', 'delete');
            deleteBtn.style.color = 'var(--mdui-color-error)';
            deleteBtn.addEventListener('click', async (e) => {
                e.stopPropagation();
                await this.deleteSubscription(group.name);
            });
            header.appendChild(deleteBtn);
        }

        // 点击展开/收起
        header.addEventListener('click', () => {
            if (this.expandedGroups.has(group.name)) {
                this.expandedGroups.delete(group.name);
            } else {
                this.expandedGroups.add(group.name);
            }
            this.update();
        });

        container.appendChild(header);

        // 展开时显示节点列表
        if (isExpanded) {
            // 获取节点详情
            const configInfos = await this.loadConfigInfos(group);

            for (const filename of group.configs) {
                const info = configInfos.get(filename) || { protocol: 'unknown', address: '', port: '' };
                const fullPath = group.dirName ? `${group.dirName}/${filename}` : filename;
                const isCurrent = currentConfig && currentConfig.endsWith(filename);

                this.renderConfigItem(container, filename, fullPath, info, isCurrent, group);
            }
        }
    }

    async loadConfigInfos(group) {
        const infoMap = new Map();

        for (const filename of group.configs) {
            try {
                const fullPath = group.dirName ? `${group.dirName}/${filename}` : filename;
                const content = await KSUService.readConfig(fullPath);
                infoMap.set(filename, this.parseOutboundInfo(content));
            } catch (e) {
                infoMap.set(filename, { protocol: 'unknown', address: '', port: '' });
            }
        }

        return infoMap;
    }

    renderConfigItem(container, filename, fullPath, info, isCurrent, group) {
        const item = document.createElement('mdui-list-item');
        item.setAttribute('clickable', '');
        item.classList.add('config-item');
        item.style.paddingLeft = '16px';

        const displayName = filename.replace(/\.json$/i, '');
        item.setAttribute('headline', displayName);

        // 使用 slot="description" 自定义多行布局
        const descContainer = document.createElement('div');
        descContainer.slot = 'description';
        descContainer.style.cssText = 'display: flex; flex-direction: column; gap: 2px; width: 100%;';

        // 第一行：协议
        const protocolLine = document.createElement('div');
        protocolLine.style.cssText = 'color: var(--mdui-color-primary); font-size: 12px;';
        protocolLine.textContent = info.protocol || '未知协议';
        descContainer.appendChild(protocolLine);

        // 第二行：地址+端口 ｜ 延迟 + 当前标识
        const addressLine = document.createElement('div');
        addressLine.style.cssText = 'display: flex; justify-content: space-between; align-items: center;';

        // 左侧：地址和端口
        const addressSpan = document.createElement('span');
        addressSpan.style.cssText = 'color: var(--mdui-color-on-surface-variant); font-size: 12px;';
        addressSpan.textContent = info.port ? `${info.address}:${info.port}` : info.address;
        addressLine.appendChild(addressSpan);

        // 右侧：状态容器（延迟 + 当前标识）
        const statusContainer = document.createElement('span');
        statusContainer.style.cssText = 'display: flex; align-items: center; gap: 6px;';

        // 延迟标签
        const latencyLabel = document.createElement('span');
        latencyLabel.className = 'latency-label';
        latencyLabel.style.cssText = 'font-size: 12px; color: var(--mdui-color-on-surface-variant);';
        statusContainer.appendChild(latencyLabel);

        // 当前配置标签
        if (isCurrent) {
            const currentTag = document.createElement('span');
            currentTag.textContent = '当前';
            currentTag.style.cssText = 'font-size: 11px; padding: 2px 6px; border-radius: 4px; background: var(--mdui-color-primary); color: var(--mdui-color-on-primary);';
            statusContainer.appendChild(currentTag);
        }

        addressLine.appendChild(statusContainer);
        descContainer.appendChild(addressLine);
        item.appendChild(descContainer);

        // 三点菜单
        const dropdown = document.createElement('mdui-dropdown');
        dropdown.setAttribute('placement', 'bottom-end');
        dropdown.slot = 'end-icon';

        const menuBtn = document.createElement('mdui-button-icon');
        menuBtn.setAttribute('slot', 'trigger');
        menuBtn.setAttribute('icon', 'more_vert');
        // 阻止所有事件冒泡到父列表项，防止触发 ripple 和选中效果
        menuBtn.addEventListener('click', (e) => {
            e.stopPropagation();
            // 关闭之前打开的下拉菜单
            if (this.currentOpenDropdown && this.currentOpenDropdown !== dropdown) {
                this.currentOpenDropdown.open = false;
            }
            // 更新当前打开的下拉菜单
            this.currentOpenDropdown = dropdown;
        });
        menuBtn.addEventListener('mousedown', (e) => e.stopPropagation());
        menuBtn.addEventListener('pointerdown', (e) => e.stopPropagation());
        menuBtn.addEventListener('touchstart', (e) => e.stopPropagation());
        dropdown.appendChild(menuBtn);

        // 监听下拉菜单关闭事件
        dropdown.addEventListener('closed', () => {
            if (this.currentOpenDropdown === dropdown) {
                this.currentOpenDropdown = null;
            }
        });

        const menu = document.createElement('mdui-menu');

        // 编辑
        const editItem = document.createElement('mdui-menu-item');
        editItem.innerHTML = '<mdui-icon slot="icon" name="edit"></mdui-icon>编辑';
        editItem.addEventListener('click', async (e) => {
            e.stopPropagation();
            dropdown.open = false;
            await this.ui.showConfigDialog(fullPath);
        });
        menu.appendChild(editItem);

        // 测试
        const testItem = document.createElement('mdui-menu-item');
        testItem.innerHTML = '<mdui-icon slot="icon" name="speed"></mdui-icon>测试';
        testItem.addEventListener('click', async (e) => {
            e.stopPropagation();
            dropdown.open = false;
            await this.testConfig(displayName, info.address, item);
        });
        menu.appendChild(testItem);

        // 删除（非当前配置可删除）
        if (!isCurrent) {
            const deleteItem = document.createElement('mdui-menu-item');
            deleteItem.innerHTML = '<mdui-icon slot="icon" name="delete"></mdui-icon>删除';
            deleteItem.style.color = 'var(--mdui-color-error)';
            deleteItem.addEventListener('click', async (e) => {
                e.stopPropagation();
                dropdown.open = false;
                await this.deleteConfig(fullPath, displayName);
            });
            menu.appendChild(deleteItem);
        }

        dropdown.appendChild(menu);
        item.appendChild(dropdown);

        item.addEventListener('click', () => {
            if (!isCurrent) {
                this.switchConfig(fullPath, displayName);
            }
        });

        container.appendChild(item);
    }

    async testConfig(displayName, address, itemElement) {
        const latencyLabel = itemElement?.querySelector('.latency-label');

        if (!address || address === '直连模式') {
            if (latencyLabel) latencyLabel.textContent = '直连';
            return;
        }

        try {
            if (latencyLabel) {
                latencyLabel.textContent = '测试中...';
                latencyLabel.style.color = 'var(--mdui-color-on-surface-variant)';
            }
            const latency = await KSUService.getPingLatency(address);
            if (latencyLabel) {
                latencyLabel.textContent = latency;
                // 根据延迟值设置颜色
                const ms = parseInt(latency);
                if (!isNaN(ms)) {
                    if (ms < 100) {
                        latencyLabel.style.color = '#4caf50'; // 绿色
                    } else if (ms < 300) {
                        latencyLabel.style.color = '#ff9800'; // 橙色
                    } else {
                        latencyLabel.style.color = '#f44336'; // 红色
                    }
                }
            }
        } catch (error) {
            if (latencyLabel) {
                latencyLabel.textContent = '失败';
                latencyLabel.style.color = '#f44336';
            }
        }
    }

    async deleteConfig(fullPath, displayName) {
        try {
            const confirmed = await this.ui.confirm(`确定要删除配置文件 "${displayName}" 吗？\n\n此操作不可恢复。`);
            if (!confirmed) return;

            const result = await KSUService.deleteConfig(fullPath);
            if (result && result.success) {
                toast('配置已删除');
                this.update();
            } else {
                toast('删除失败: ' + (result?.error || '未知错误'));
            }
        } catch (error) {
            toast('删除失败: ' + error.message);
        }
    }

    async switchConfig(fullPath, displayName) {
        try {
            await KSUService.switchConfig(fullPath);
            toast('已切换到: ' + displayName);
            await this.update();
            await this.ui.statusPage.update();
        } catch (error) {
            toast('切换配置失败: ' + error.message);
        }
    }

    // ===================== 订阅管理 =====================

    async updateSubscription(name) {
        toast(`正在更新订阅 "${name}"...`);

        // 先显示骨架屏
        const listEl = document.getElementById('config-list');
        this.ui.showSkeleton(listEl, 5, { showIcon: false });

        // 使用 setTimeout 让浏览器先渲染 UI
        setTimeout(async () => {
            try {
                await KSUService.updateSubscription(name);
                toast('订阅更新成功');
                this.update();
            } catch (error) {
                toast('更新失败: ' + error.message);
                this.update();
            }
        }, 50);
    }

    async deleteSubscription(name) {
        try {
            const confirmed = await this.ui.confirm(`确定要删除订阅 "${name}" 吗？\n\n该订阅下的所有节点都将被删除。`);
            if (!confirmed) return;

            await KSUService.removeSubscription(name);
            toast('订阅已删除');
            this.expandedGroups.delete(name);
            this.update();
        } catch (error) {
            toast('删除失败: ' + error.message);
        }
    }

    async addSubscription() {
        const dialog = document.getElementById('subscription-dialog');
        const nameInput = document.getElementById('subscription-name');
        const urlInput = document.getElementById('subscription-url');

        nameInput.value = '';
        urlInput.value = '';
        dialog.open = true;
    }

    async saveSubscription() {
        const nameInput = document.getElementById('subscription-name');
        const urlInput = document.getElementById('subscription-url');
        const saveBtn = document.getElementById('subscription-save');
        const cancelBtn = document.getElementById('subscription-cancel');
        const dialog = document.getElementById('subscription-dialog');

        const name = nameInput.value.trim();
        const url = urlInput.value.trim();

        if (!name) {
            toast('请输入订阅名称');
            return;
        }

        if (!url) {
            toast('请输入订阅地址');
            return;
        }

        // 关闭对话框
        dialog.open = false;

        // 清空输入
        nameInput.value = '';
        urlInput.value = '';

        // 显示骨架屏
        const listEl = document.getElementById('config-list');
        this.ui.showSkeleton(listEl, 5, { showIcon: false });

        toast('正在下载订阅，请稍候...');

        // 使用 setTimeout 让浏览器先渲染 UI，再执行阻塞操作
        setTimeout(async () => {
            try {
                await KSUService.addSubscription(name, url);
                toast('订阅添加成功');
                this.expandedGroups.add(name);
                this.update();
            } catch (error) {
                toast('添加失败: ' + error.message);
                this.update();
            }
        }, 50);
    }

    // ===================== 原有方法 =====================

    async showDialog(filename = null) {
        const dialog = document.getElementById('config-dialog');
        const filenameInput = document.getElementById('config-filename');
        const contentInput = document.getElementById('config-content');

        if (filename) {
            filenameInput.value = filename;
            filenameInput.disabled = true;
            const content = await KSUService.readConfig(filename);
            contentInput.value = content;
        } else {
            filenameInput.value = '';
            filenameInput.disabled = false;
            contentInput.value = JSON.stringify({
                "outbounds": [
                    {
                        "protocol": "vless",
                        "tag": "proxy",
                        "settings": {
                            "vnext": [{ "address": "", "port": 443, "users": [{ "id": "" }] }]
                        }
                    },
                    { "protocol": "freedom", "tag": "direct" },
                    { "protocol": "blackhole", "tag": "block" }
                ]
            }, null, 2);
        }

        dialog.open = true;
    }

    async saveConfig() {
        const filename = document.getElementById('config-filename').value.trim();
        const content = document.getElementById('config-content').value;

        if (!filename) {
            toast('请输入文件名');
            return;
        }

        if (!filename.endsWith('.json')) {
            toast('文件名必须以 .json 结尾');
            return;
        }

        try {
            JSON.parse(content);
            await KSUService.saveConfig(filename, content);
            toast('保存成功');
            document.getElementById('config-dialog').open = false;
            this.update();
        } catch (error) {
            toast('保存失败: ' + error.message);
        }
    }

    async importNodeLink() {
        const input = document.getElementById('node-link-input');
        const nodeLink = input.value.trim();

        if (!nodeLink) {
            toast('请输入节点链接');
            return;
        }

        const supportedProtocols = ['vless://', 'vmess://', 'trojan://', 'ss://', 'socks://', 'http://', 'https://'];
        const isValid = supportedProtocols.some(protocol => nodeLink.startsWith(protocol));

        if (!isValid) {
            toast('不支持的节点链接格式');
            return;
        }

        try {
            toast('正在导入节点...');
            const result = await KSUService.importFromNodeLink(nodeLink);

            if (result.success) {
                toast('节点导入成功');
                document.getElementById('node-link-dialog').open = false;
                input.value = '';
                this.update();
            } else {
                toast('导入失败: ' + (result.error || '未知错误'));
            }
        } catch (error) {
            toast('导入失败: ' + error.message);
        }
    }
}
