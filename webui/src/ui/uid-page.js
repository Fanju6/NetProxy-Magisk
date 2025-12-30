import { KSUService } from '../services/ksu-service.js';
import { toast } from '../utils/toast.js';

/**
 * 代理设置页面管理器
 */
export class UIDPageManager {
    constructor(ui) {
        this.ui = ui;
        this.allApps = [];
        this.proxyMode = 'blacklist';
        this.proxyApps = [];
        this.selectedApps = new Map(); // 用于多选
    }

    async init() {
        // 绑定模式切换事件
        const modeSwitch = document.getElementById('proxy-mode-switch');
        if (modeSwitch) {
            modeSwitch.addEventListener('change', async (e) => {
                const newMode = e.target.value;
                await this.setProxyMode(newMode);
            });
        }
    }

    async update() {
        try {
            const listEl = document.getElementById('uid-list');
            const modeSwitch = document.getElementById('proxy-mode-switch');
            const modeDesc = document.getElementById('proxy-mode-desc');
            const listTitle = document.getElementById('proxy-list-title');

            // 显示骨架屏
            const currentCount = listEl.children.length > 0 ? listEl.children.length : 1;
            this.ui.showSkeleton(listEl, currentCount);

            // 获取代理模式
            this.proxyMode = await KSUService.getAppProxyMode();

            // 更新模式开关和描述
            if (modeSwitch) {
                modeSwitch.value = this.proxyMode;
            }

            if (modeDesc) {
                if (this.proxyMode === 'blacklist') {
                    modeDesc.textContent = '黑名单模式：代理所有应用，排除列表中的应用';
                } else {
                    modeDesc.textContent = '白名单模式：仅代理列表中的应用';
                }
            }

            if (listTitle) {
                listTitle.textContent = this.proxyMode === 'blacklist' ? '排除应用' : '代理应用';
            }

            // 获取代理应用列表
            this.proxyApps = await KSUService.getProxyApps();

            if (this.proxyApps.length === 0) {
                const emptyText = this.proxyMode === 'blacklist'
                    ? '暂无排除应用，所有应用都会走代理'
                    : '暂无代理应用，点击上方按钮添加';
                listEl.innerHTML = `<mdui-list-item><div slot="headline">${emptyText}</div></mdui-list-item>`;
                return;
            }

            // 获取所有应用信息以便匹配包名
            let allApps = [];
            try {
                allApps = await KSUService.getInstalledApps();
            } catch (e) {
                console.warn('Failed to load app info:', e);
            }

            // 创建包名到应用的映射
            const pkgToApp = {};
            allApps.forEach(app => {
                pkgToApp[app.packageName] = app;
            });

            // 图标懒加载观察器 - 支持 KSU API 和 WebUI X 两种方式
            const observer = new IntersectionObserver((entries) => {
                entries.forEach(entry => {
                    if (entry.isIntersecting) {
                        const item = entry.target;
                        const img = item.querySelector('img.app-icon');
                        if (img && !img.src) {
                            const iconUrl = img.dataset.iconUrl;
                            const packageName = img.dataset.packageName;

                            if (iconUrl) {
                                // KSU API 方式：直接使用 ksu://icon/ URL
                                img.src = iconUrl;
                                img.onload = function () {
                                    this.style.display = 'block';
                                    const placeholder = item.querySelector('mdui-icon[slot="icon"]');
                                    if (placeholder) {
                                        placeholder.style.display = 'none';
                                    }
                                };
                            } else if (packageName) {
                                // WebUI X 方式：通过 $packageManager 加载
                                KSUService.loadAppIcon(packageName).then(base64 => {
                                    if (base64) {
                                        img.src = base64;
                                        img.style.display = 'block';
                                        const placeholder = item.querySelector('mdui-icon[slot="icon"]');
                                        if (placeholder) {
                                            placeholder.style.display = 'none';
                                        }
                                    }
                                });
                            }
                        }
                        observer.unobserve(item);
                    }
                });
            }, {
                rootMargin: '100px',
                threshold: 0.1
            });

            listEl.innerHTML = '';
            this.proxyApps.forEach(packageName => {
                const item = document.createElement('mdui-list-item');
                const app = pkgToApp[packageName];

                if (app) {
                    item.setAttribute('headline', app.appLabel);

                    // 使用自定义 slot 显示包名，以便控制换行样式
                    const descSpan = document.createElement('span');
                    descSpan.slot = 'description';
                    descSpan.className = 'package-name-wrap';
                    descSpan.textContent = packageName;
                    item.appendChild(descSpan);

                    // 统一使用懒加载方式
                    const icon = document.createElement('mdui-icon');
                    icon.slot = 'icon';
                    icon.setAttribute('name', 'android');
                    item.appendChild(icon);

                    const iconEl = document.createElement('img');
                    iconEl.slot = 'icon';
                    iconEl.className = 'app-icon';
                    iconEl.style.display = 'none';

                    if (app.icon) {
                        // KSU API 方式：有 ksu://icon/ URL
                        iconEl.dataset.iconUrl = app.icon;
                    }
                    // WebUI X 方式：通过包名懒加载
                    iconEl.dataset.packageName = packageName;

                    iconEl.onerror = function () {
                        this.style.display = 'none';
                        const placeholder = this.parentElement.querySelector('mdui-icon[slot="icon"]');
                        if (placeholder) {
                            placeholder.style.display = '';
                        }
                    };

                    item.appendChild(iconEl);
                    observer.observe(item);
                } else {
                    item.setAttribute('headline', packageName);
                    item.setAttribute('description', '未安装或无法识别');
                    item.setAttribute('icon', 'android');
                }

                // 添加删除按钮
                const deleteBtn = document.createElement('mdui-button-icon');
                deleteBtn.slot = 'end-icon';
                deleteBtn.setAttribute('icon', 'delete');
                deleteBtn.style.color = 'var(--mdui-color-error)';
                deleteBtn.addEventListener('click', async (e) => {
                    e.stopPropagation();
                    const appName = app ? app.appLabel : packageName;
                    await this.removeApp(packageName, appName);
                });
                item.appendChild(deleteBtn);

                listEl.appendChild(item);
            });
        } catch (error) {
            console.error('Update proxy page failed:', error);
        }
    }

    async setProxyMode(mode) {
        try {
            await KSUService.setAppProxyMode(mode);
            this.proxyMode = mode;

            // 更新 UI
            const modeDesc = document.getElementById('proxy-mode-desc');
            const listTitle = document.getElementById('proxy-list-title');

            if (modeDesc) {
                if (mode === 'blacklist') {
                    modeDesc.textContent = '黑名单模式：代理所有应用，排除列表中的应用';
                } else {
                    modeDesc.textContent = '白名单模式：仅代理列表中的应用';
                }
            }

            if (listTitle) {
                listTitle.textContent = mode === 'blacklist' ? '排除应用' : '代理应用';
            }

            // 检查服务状态并给出相应提示
            const { status } = await KSUService.getStatus();
            const modeName = mode === 'blacklist' ? '黑名单' : '白名单';
            if (status === 'running') {
                toast(`已切换到${modeName}模式，重启服务后生效`);
            } else {
                toast(`已切换到${modeName}模式`);
            }

            this.update();
        } catch (error) {
            toast('切换模式失败: ' + error.message, true);
        }
    }

    async removeApp(packageName, appName) {
        if (await this.ui.confirm(`确定要移除 ${appName} 吗？`)) {
            try {
                await KSUService.removeProxyApp(packageName);

                // 检查服务状态并给出相应提示
                const { status } = await KSUService.getStatus();
                if (status === 'running') {
                    toast(`已移除 ${appName}，重启服务后生效`);
                } else {
                    toast(`已移除 ${appName}`);
                }

                this.update();
            } catch (error) {
                toast('移除失败: ' + error.message, true);
            }
        }
    }

    async showAppSelector() {
        const dialog = document.getElementById('app-selector-dialog');
        const listEl = document.getElementById('app-selector-list');
        const addSelectedBtn = document.getElementById('app-selector-add-selected');

        // 清空选中状态
        this.selectedApps.clear();
        this.updateAddSelectedButton();

        // 绑定批量添加按钮事件（每次打开都重新绑定）
        if (addSelectedBtn) {
            addSelectedBtn.onclick = () => this.addSelectedApps();
        }

        dialog.open = true;

        // 显示骨架屏
        this.ui.showSkeleton(listEl, 5);

        try {
            this.allApps = await KSUService.getInstalledApps();
            this.renderAppList(this.allApps);
        } catch (error) {
            listEl.innerHTML = '<mdui-list-item><div slot="headline">加载失败</div></mdui-list-item>';
            toast('加载应用列表失败: ' + error.message, true);
        }
    }

    updateAddSelectedButton() {
        const btn = document.getElementById('app-selector-add-selected');
        if (btn) {
            const count = this.selectedApps.size;
            btn.textContent = `添加选中 (${count})`;
            btn.disabled = count === 0;
        }
    }

    toggleAppSelection(app, checkbox, fromCheckbox = false) {
        if (fromCheckbox) {
            // 从复选框触发：复选框已经自动切换了状态，直接根据当前状态更新数据
            if (checkbox.checked) {
                this.selectedApps.set(app.packageName, app);
            } else {
                this.selectedApps.delete(app.packageName);
            }
        } else {
            // 从列表项触发：需要手动切换复选框状态
            if (this.selectedApps.has(app.packageName)) {
                this.selectedApps.delete(app.packageName);
                checkbox.checked = false;
            } else {
                this.selectedApps.set(app.packageName, app);
                checkbox.checked = true;
            }
        }
        this.updateAddSelectedButton();
    }

    async addSelectedApps() {
        if (this.selectedApps.size === 0) return;

        const apps = Array.from(this.selectedApps.values());

        for (const app of apps) {
            try {
                await KSUService.addProxyApp(app.packageName);
            } catch (error) {
                // 忽略错误（如应用已存在）
            }
        }

        // 检查服务状态并给出相应提示
        const { status } = await KSUService.getStatus();
        if (status === 'running') {
            toast(`成功添加 ${apps.length} 个应用，重启服务后生效`);
        } else {
            toast(`成功添加 ${apps.length} 个应用`);
        }

        document.getElementById('app-selector-dialog').open = false;
        this.selectedApps.clear();
        this.update();
    }

    renderAppList(apps) {
        // 清空之前的图标加载队列
        KSUService.clearIconLoadQueue();

        const listEl = document.getElementById('app-selector-list');
        // 获取滚动容器作为 IntersectionObserver 的 root
        const scrollContainer = listEl.parentElement;

        if (apps.length === 0) {
            listEl.innerHTML = '<mdui-list-item><div slot="headline">没有找到应用</div></mdui-list-item>';
            return;
        }

        listEl.innerHTML = '';

        const observer = new IntersectionObserver((entries) => {
            entries.forEach(entry => {
                if (entry.isIntersecting) {
                    const item = entry.target;
                    const img = item.querySelector('img.app-icon');
                    if (img && !img.src) {
                        const iconUrl = img.dataset.iconUrl;
                        const packageName = img.dataset.packageName;

                        if (iconUrl) {
                            // KSU API 方式：直接使用 ksu://icon/ URL
                            img.src = iconUrl;
                            img.onload = function () {
                                this.style.display = 'block';
                                const placeholder = item.querySelector('mdui-icon[slot="icon"]');
                                if (placeholder) {
                                    placeholder.style.display = 'none';
                                }
                            };
                        } else if (packageName) {
                            // WebUI X 方式：通过 $packageManager 加载
                            KSUService.loadAppIcon(packageName).then(base64 => {
                                if (base64) {
                                    img.src = base64;
                                    img.style.display = 'block';
                                    const placeholder = item.querySelector('mdui-icon[slot="icon"]');
                                    if (placeholder) {
                                        placeholder.style.display = 'none';
                                    }
                                }
                            });
                        }
                    }
                    observer.unobserve(item);
                }
            });
        }, {
            root: scrollContainer,
            rootMargin: '50px',
            threshold: 0.1
        });

        apps.forEach(app => {
            const item = document.createElement('mdui-list-item');
            item.setAttribute('clickable', '');
            item.setAttribute('headline', app.appLabel);
            item.setAttribute('description', app.packageName);

            // 添加应用图标 - 统一使用懒加载
            const icon = document.createElement('mdui-icon');
            icon.slot = 'icon';
            icon.setAttribute('name', 'android');
            item.appendChild(icon);

            const iconEl = document.createElement('img');
            iconEl.slot = 'icon';
            iconEl.className = 'app-icon';
            iconEl.style.display = 'none';

            if (app.icon) {
                // KSU API 方式：有 ksu://icon/ URL，也使用懒加载
                iconEl.dataset.iconUrl = app.icon;
            }
            // WebUI X 方式：通过包名懒加载
            iconEl.dataset.packageName = app.packageName;

            iconEl.onerror = function () {
                this.style.display = 'none';
                const placeholder = this.parentElement.querySelector('mdui-icon[slot="icon"]');
                if (placeholder) {
                    placeholder.style.display = '';
                }
            };

            item.appendChild(iconEl);
            observer.observe(item);

            // 添加复选框
            const checkbox = document.createElement('mdui-checkbox');
            checkbox.slot = 'end-icon';
            checkbox.checked = this.selectedApps.has(app.packageName);
            item.appendChild(checkbox);

            // 复选框变化事件 - 阻止冒泡并同步选中状态
            checkbox.addEventListener('change', (e) => {
                e.stopPropagation();
                this.toggleAppSelection(app, checkbox, true);
            });

            // 点击整行切换选中状态（排除复选框区域）
            item.addEventListener('click', (e) => {
                // 如果点击的是复选框本身，不处理（让 change 事件处理）
                if (e.target === checkbox || checkbox.contains(e.target)) {
                    return;
                }
                e.stopPropagation();
                this.toggleAppSelection(app, checkbox, false);
            });

            listEl.appendChild(item);
        });
    }


    filterApps(query) {
        if (!this.allApps) return;

        const filtered = this.allApps.filter(app =>
            app.appLabel.toLowerCase().includes(query.toLowerCase()) ||
            app.packageName.toLowerCase().includes(query.toLowerCase())
        );

        this.renderAppList(filtered);
    }
}
