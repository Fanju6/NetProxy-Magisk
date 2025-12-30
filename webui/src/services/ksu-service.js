import { exec, listPackages, getPackagesInfo } from 'kernelsu';
import { wrapInputStream } from 'webuix';
import { toast } from '../utils/toast.js';

/**
 * KernelSU Service - 封装与 KernelSU API 的交互
 */
export class KSUService {
    static MODULE_PATH = '/data/adb/modules/netproxy';

    static async exec(command, options = {}) {
        try {
            const { errno, stdout, stderr } = await exec(command, options);
            if (errno !== 0) {
                throw new Error(stderr || 'Command execution failed');
            }
            return stdout.trim();
        } catch (error) {
            console.error('KSU exec error:', error);
            toast(error.message);
            throw error;
        }
    }

    // 获取服务状态
    static async getStatus() {
        try {
            const output = await this.exec(`cat ${this.MODULE_PATH}/config/status.conf`);
            const status = output.match(/status="([^"]+)"/)?.[1] || 'unknown';
            const config = output.match(/config="([^"]+)"/)?.[1] || '';
            return { status, config: config.split('/').pop() };
        } catch (error) {
            return { status: 'unknown', config: '' };
        }
    }

    // 启动服务
    static async startService() {
        await exec(`su -c "sh ${this.MODULE_PATH}/scripts/core/start.sh"`);
    }

    // 停止服务
    static async stopService() {
        await exec(`su -c "sh ${this.MODULE_PATH}/scripts/core/stop.sh"`);
    }

    // 获取配置文件列表（从 outbounds 目录）
    static async getConfigList() {
        try {
            const output = await this.exec(`ls ${this.MODULE_PATH}/config/xray/outbounds/*.json 2>/dev/null || echo`);
            return output.split('\n').filter(f => f).map(f => f.split('/').pop());
        } catch (error) {
            return [];
        }
    }

    static async deleteConfig(filename) {
        console.log('>>> KSUService.deleteConfig START, filename:', filename);
        try {
            const cmd = `su -c "rm '${this.MODULE_PATH}/config/xray/outbounds/${filename}'"`;
            console.log('>>> Executing command:', cmd);
            await exec(cmd);
            console.log('>>> Delete successful (no exception)');
            return { success: true };
        } catch (error) {
            console.error('>>> deleteConfig exception:', error);
            return { success: false, error: error.message };
        }
    }

    static async deleteUID(uid) {
        console.log('>>> KSUService.deleteUID START, uid:', uid);
        try {
            const uidListPath = `${this.MODULE_PATH}/config/uid_list.conf`;
            const cmd = `su -c "sed -i '/^${uid}$/d' '${uidListPath}'"`;
            console.log('>>> Executing command:', cmd);
            await exec(cmd);
            console.log('>>> Delete UID successful (no exception)');
            return { success: true };
        } catch (error) {
            console.error('>>> deleteUID exception:', error);
            return { success: false, error: error.message };
        }
    }

    // 即时应用iptables规则（添加UID）
    static async applyUIDIptables(uid) {
        try {
            console.log('Applying iptables rule for UID:', uid);
            const cmd = `su -c "iptables -t nat -I OUTPUT -p tcp -m owner --uid-owner ${uid} -j RETURN"`;
            await exec(cmd);
            console.log('Iptables rule applied for UID:', uid);
            return { success: true };
        } catch (error) {
            console.error('Failed to apply iptables rule:', error);
            return { success: false, error: error.message };
        }
    }

    // 即时删除iptables规则（删除UID）
    static async removeUIDIptables(uid) {
        try {
            console.log('Removing iptables rule for UID:', uid);
            const cmd = `su -c "iptables -t nat -D OUTPUT -p tcp -m owner --uid-owner ${uid} -j RETURN"`;
            await exec(cmd);
            console.log('Iptables rule removed for UID:', uid);
            return { success: true };
        } catch (error) {
            console.error('Failed to remove iptables rule:', error);
            return { success: false, error: error.message };
        }
    }

    // 读取配置文件（从 outbounds 目录）
    static async readConfig(filename) {
        return await this.exec(`cat '${this.MODULE_PATH}/config/xray/outbounds/${filename}'`);
    }

    // 保存配置文件（到 outbounds 目录）
    static async saveConfig(filename, content) {
        const escaped = content.replace(/'/g, "'\\''");
        await this.exec(`echo '${escaped}' > '${this.MODULE_PATH}/config/xray/outbounds/${filename}'`);
    }

    // 从节点链接导入配置
    static async importFromNodeLink(nodeLink) {
        try {
            console.log('Importing from node link...');
            const cmd = `su -c "${this.MODULE_PATH}/scripts/config/url2json.sh '${nodeLink}'"`;
            const result = await exec(cmd);
            console.log('Import result:', result);

            if (result.errno === 0) {
                return { success: true, output: result.stdout };
            } else {
                return { success: false, error: result.stderr || 'Import failed' };
            }
        } catch (error) {
            console.error('Import from node link error:', error);
            return { success: false, error: error.message };
        }
    }

    // 获取 Xray 版本号
    static async getXrayVersion() {
        try {
            const result = await exec(`${this.MODULE_PATH}/bin/xray version`);
            if (result.errno === 0) {
                const match = result.stdout.match(/Xray\s+([\d.]+)/);
                return match ? match[1] : 'unknown';
            }
            return 'unknown';
        } catch (error) {
            console.error('Failed to get Xray version:', error);
            return 'unknown';
        }
    }

    // 检查并更新 Xray 内核
    static async updateXray() {
        try {
            const cmd = `su -c "sh ${this.MODULE_PATH}/scripts/utils/update-xray.sh"`;
            const result = await exec(cmd);

            if (result.errno === 0) {
                const output = (result.stdout || '') + (result.stderr || '');
                console.log('Update output:', output);

                if (output.includes('已是最新版本') || output.includes('无需更新')) {
                    return { success: true, isLatest: true, message: '已是最新版本，无需更新', output };
                } else if (output.includes('更新成功') || output.includes('========== 更新成功')) {
                    return { success: true, isLatest: false, message: '更新成功', output };
                } else {
                    return { success: true, isLatest: false, message: '操作完成', output };
                }
            } else {
                return { success: false, isLatest: false, message: '更新失败', error: result.stderr };
            }
        } catch (error) {
            console.error('Update Xray error:', error);
            return { success: false, isLatest: false, message: '更新失败', error: error.message };
        }
    }

    // 切换配置（支持热切换）
    static async switchConfig(filename) {
        const configPath = `${this.MODULE_PATH}/config/xray/outbounds/${filename}`;
        const { status } = await this.getStatus();

        if (status === 'running') {
            // 热切换：使用 Xray API 动态更新出站配置
            const result = await exec(`su -c "sh ${this.MODULE_PATH}/scripts/core/switch-config.sh '${configPath}'"`);
            if (result.errno !== 0) {
                throw new Error(result.stderr || '热切换失败');
            }
        } else {
            // 服务未运行：只更新 status.conf
            const newStatus = `status="stopped"\nconfig="${configPath}"`;
            await this.exec(`echo '${newStatus}' > ${this.MODULE_PATH}/config/status.conf`);
        }
    }

    // 获取分应用代理模式 (blacklist/whitelist)
    static async getAppProxyMode() {
        try {
            const content = await this.exec(`cat ${this.MODULE_PATH}/config/tproxy.conf`);
            const match = content.match(/^APP_PROXY_MODE="?(\w+)"?/m);
            return match ? match[1] : 'blacklist';
        } catch (error) {
            return 'blacklist';
        }
    }

    // 设置分应用代理模式
    static async setAppProxyMode(mode) {
        await this.exec(`sed -i 's/^APP_PROXY_MODE=.*/APP_PROXY_MODE="${mode}"/' ${this.MODULE_PATH}/config/tproxy.conf`);
    }

    // 获取代理应用列表（包名）- 根据模式返回 BYPASS 或 PROXY 列表
    static async getProxyApps() {
        try {
            const content = await this.exec(`cat ${this.MODULE_PATH}/config/tproxy.conf`);
            const mode = (content.match(/APP_PROXY_MODE="?(\w+)"?/) || [])[1] || 'blacklist';

            // 黑名单模式用 BYPASS_APPS_LIST，白名单模式用 PROXY_APPS_LIST
            const listKey = mode === 'blacklist' ? 'BYPASS_APPS_LIST' : 'PROXY_APPS_LIST';
            const match = content.match(new RegExp(`${listKey}="([^"]*)"`));

            if (match && match[1]) {
                return match[1].split(' ').filter(pkg => pkg.trim());
            }
            return [];
        } catch (error) {
            return [];
        }
    }

    // 添加代理应用
    static async addProxyApp(packageName) {
        const content = await this.exec(`cat ${this.MODULE_PATH}/config/tproxy.conf`);
        const mode = (content.match(/APP_PROXY_MODE="?(\w+)"?/) || [])[1] || 'blacklist';
        const listKey = mode === 'blacklist' ? 'BYPASS_APPS_LIST' : 'PROXY_APPS_LIST';
        const match = content.match(new RegExp(`${listKey}="([^"]*)"`));
        const currentList = match ? match[1] : '';

        if (currentList.split(' ').includes(packageName)) {
            throw new Error('应用已存在');
        }

        const newList = currentList ? `${currentList} ${packageName}` : packageName;
        await this.exec(`sed -i 's/${listKey}="[^"]*"/${listKey}="${newList}"/' ${this.MODULE_PATH}/config/tproxy.conf`);
    }

    // 删除代理应用
    static async removeProxyApp(packageName) {
        const content = await this.exec(`cat ${this.MODULE_PATH}/config/tproxy.conf`);
        const mode = (content.match(/APP_PROXY_MODE="?(\w+)"?/) || [])[1] || 'blacklist';
        const listKey = mode === 'blacklist' ? 'BYPASS_APPS_LIST' : 'PROXY_APPS_LIST';
        const match = content.match(new RegExp(`${listKey}="([^"]*)"`));
        const currentList = match ? match[1] : '';

        const newList = currentList.split(' ').filter(pkg => pkg !== packageName).join(' ');
        await this.exec(`sed -i 's/${listKey}="[^"]*"/${listKey}="${newList}"/' ${this.MODULE_PATH}/config/tproxy.conf`);
    }

    // 刷新 TProxy 规则（用于配置变更后即时生效）
    static async renewTProxy() {
        try {
            const result = await exec(`su -c "sh ${this.MODULE_PATH}/scripts/network/tproxy.sh restart"`);
            return { success: result.errno === 0, output: result.stdout };
        } catch (error) {
            return { success: false, error: error.message };
        }
    }

    // ===================== 订阅管理 =====================

    // 获取所有订阅列表
    static async getSubscriptions() {
        try {
            const result = await this.exec(`find ${this.MODULE_PATH}/config/xray/outbounds -mindepth 1 -maxdepth 1 -type d -name 'sub_*' -exec basename {} \\;`);
            const subscriptions = [];

            for (const dir of result.split('\n').filter(d => d)) {
                const name = dir.replace(/^sub_/, '');
                try {
                    const metaContent = await this.exec(`cat ${this.MODULE_PATH}/config/xray/outbounds/${dir}/_meta.json`);
                    const meta = JSON.parse(metaContent);
                    const nodeCount = await this.exec(`find ${this.MODULE_PATH}/config/xray/outbounds/${dir} -name '*.json' ! -name '_meta.json' | wc -l`);
                    subscriptions.push({
                        name: meta.name || name,
                        dirName: dir,
                        url: meta.url,
                        updated: meta.updated,
                        nodeCount: parseInt(nodeCount.trim()) || 0
                    });
                } catch (e) {
                    // 无效订阅目录
                }
            }
            return subscriptions;
        } catch (error) {
            return [];
        }
    }

    // 添加订阅（后台执行，不阻塞 UI）
    static async addSubscription(name, url) {
        const statusFile = `${this.MODULE_PATH}/config/.sub_status`;

        // 清除旧状态文件
        await this.exec(`rm -f ${statusFile}`);

        // 后台运行脚本，完成后写入状态
        await exec(`su -c "nohup sh -c 'sh ${this.MODULE_PATH}/scripts/config/subscription.sh add \\"${name}\\" \\"${url}\\" && echo success > ${statusFile} || echo fail > ${statusFile}' > /dev/null 2>&1 &"`);

        // 轮询等待完成（最多 60 秒）
        return await this.waitForSubscriptionComplete(statusFile, 60000);
    }

    // 更新订阅（后台执行，不阻塞 UI）
    static async updateSubscription(name) {
        const statusFile = `${this.MODULE_PATH}/config/.sub_status`;

        // 清除旧状态文件
        await this.exec(`rm -f ${statusFile}`);

        // 后台运行脚本
        await exec(`su -c "nohup sh -c 'sh ${this.MODULE_PATH}/scripts/config/subscription.sh update \\"${name}\\" && echo success > ${statusFile} || echo fail > ${statusFile}' > /dev/null 2>&1 &"`);

        // 轮询等待完成
        return await this.waitForSubscriptionComplete(statusFile, 60000);
    }

    // 轮询等待订阅操作完成
    static async waitForSubscriptionComplete(statusFile, timeout) {
        const startTime = Date.now();
        const pollInterval = 500; // 每 500ms 检查一次

        while (Date.now() - startTime < timeout) {
            await new Promise(resolve => setTimeout(resolve, pollInterval));

            // 使用 exec 直接调用，检查返回值而不是抛出异常
            const result = await exec(`cat ${statusFile} 2>/dev/null || echo ""`);
            const status = (result.stdout || '').trim();

            if (status === 'success') {
                await exec(`rm -f ${statusFile}`);
                return { success: true };
            } else if (status === 'fail') {
                await exec(`rm -f ${statusFile}`);
                throw new Error('订阅操作失败');
            }
            // 状态文件不存在或为空，继续等待
        }

        throw new Error('操作超时');
    }

    // 删除订阅
    static async removeSubscription(name) {
        const result = await exec(`su -c "sh ${this.MODULE_PATH}/scripts/config/subscription.sh remove '${name}'"`);
        if (result.errno !== 0) {
            throw new Error(result.stderr || '删除订阅失败');
        }
        return { success: true };
    }

    // 获取分组配置（包含默认分组和订阅分组）
    static async getConfigGroups() {
        const groups = [];
        const outboundsDir = `${this.MODULE_PATH}/config/xray/outbounds`;

        // 获取默认分组（直接在 outbounds 目录下的 json 文件）
        try {
            const defaultFiles = await this.exec(`find ${outboundsDir} -maxdepth 1 -name '*.json' -exec basename {} \\;`);
            const defaultConfigs = defaultFiles.split('\n').filter(f => f);
            if (defaultConfigs.length > 0) {
                groups.push({
                    type: 'default',
                    name: '默认分组',
                    dirName: '',
                    configs: defaultConfigs
                });
            }
        } catch (e) { }

        // 获取订阅分组
        const subscriptions = await this.getSubscriptions();
        for (const sub of subscriptions) {
            try {
                const files = await this.exec(`find ${outboundsDir}/${sub.dirName} -name '*.json' ! -name '_meta.json' -exec basename {} \\;`);
                groups.push({
                    type: 'subscription',
                    name: sub.name,
                    dirName: sub.dirName,
                    url: sub.url,
                    updated: sub.updated,
                    configs: files.split('\n').filter(f => f)
                });
            } catch (e) { }
        }

        return groups;
    }

    // 获取日志
    static async getServiceLog(lines = 100) {
        try {
            return await this.exec(`tail -n ${lines} ${this.MODULE_PATH}/logs/service.log`);
        } catch (error) {
            return '暂无日志';
        }
    }

    static async getXrayLog(lines = 100) {
        try {
            return await this.exec(`tail -n ${lines} ${this.MODULE_PATH}/logs/xray.log`);
        } catch (error) {
            return '暂无日志';
        }
    }

    static async getTproxyLog(lines = 100) {
        try {
            return await this.exec(`tail -n ${lines} ${this.MODULE_PATH}/logs/tproxy.log`);
        } catch (error) {
            return '暂无日志';
        }
    }

    static async getUpdateLog(lines = 100) {
        try {
            return await this.exec(`tail -n ${lines} ${this.MODULE_PATH}/logs/update.log`);
        } catch (error) {
            return '暂无日志';
        }
    }

    // 获取服务运行时间
    static async getUptime() {
        try {
            console.log('getUptime: trying ps command...');
            const result = await exec(`ps -o etime= -C xray 2>/dev/null | head -1 | tr -d ' '`);
            console.log('getUptime: method 1 - errno:', result.errno, 'stdout:', result.stdout);

            if (result.errno === 0 && result.stdout.trim()) {
                return result.stdout.trim();
            }

            console.log('getUptime: method 1 failed, trying fallback...');
            const fallback = await exec(`ps -eo etime,comm | grep xray | grep -v grep | head -1 | awk '{print $1}'`);
            console.log('getUptime: method 2 - errno:', fallback.errno, 'stdout:', fallback.stdout);

            if (fallback.errno === 0 && fallback.stdout.trim()) {
                return fallback.stdout.trim();
            }

            console.warn('getUptime: both methods failed');
            return '--';
        } catch (error) {
            console.error('getUptime: error -', error);
            return '--';
        }
    }

    // 获取实时网速
    static async getNetworkSpeed() {
        try {
            const rx1Result = await exec(`awk '/:/ {sum+=$2} END {print sum}' /proc/net/dev`);
            const tx1Result = await exec(`awk '/:/ {sum+=$10} END {print sum}' /proc/net/dev`);

            if (rx1Result.errno !== 0 || tx1Result.errno !== 0) {
                return { download: '0 KB/s', upload: '0 KB/s' };
            }

            const rx1 = parseInt(rx1Result.stdout.trim()) || 0;
            const tx1 = parseInt(tx1Result.stdout.trim()) || 0;

            await new Promise(resolve => setTimeout(resolve, 1000));

            const rx2Result = await exec(`awk '/:/ {sum+=$2} END {print sum}' /proc/net/dev`);
            const tx2Result = await exec(`awk '/:/ {sum+=$10} END {print sum}' /proc/net/dev`);

            const rx2 = parseInt(rx2Result.stdout.trim()) || 0;
            const tx2 = parseInt(tx2Result.stdout.trim()) || 0;

            const downloadSpeed = Math.max(0, Math.floor((rx2 - rx1) / 1024));
            const uploadSpeed = Math.max(0, Math.floor((tx2 - tx1) / 1024));

            return { download: `${downloadSpeed} KB/s`, upload: `${uploadSpeed} KB/s` };
        } catch (error) {
            console.error('Failed to get network speed:', error);
            return { download: '0 KB/s', upload: '0 KB/s' };
        }
    }

    // 获取Xray内存占用
    static async getMemoryUsage() {
        try {
            const result = await exec(`ps -o rss,comm | grep xray | grep -v grep | awk '{sum+=$1} END {print sum}'`);
            if (result.errno !== 0 || !result.stdout || result.stdout.trim() === '') {
                return '--';
            }

            const memoryKB = parseInt(result.stdout.trim()) || 0;
            if (memoryKB === 0) return '--';

            if (memoryKB > 1024) {
                return `${(memoryKB / 1024).toFixed(1)} MB`;
            } else {
                return `${memoryKB} KB`;
            }
        } catch (error) {
            console.error('Get memory usage error:', error);
            return '--';
        }
    }

    // 获取ping延迟
    static async getPingLatency(host) {
        try {
            const result = await exec(`ping -c 1 -W 1 ${host} 2>&1 | grep 'time=' | awk -F 'time=' '{print $2}' | awk '{print $1}'`);

            if (result.errno === 0 && result.stdout.trim()) {
                const latency = parseFloat(result.stdout.trim());
                if (!isNaN(latency)) {
                    return `${Math.round(latency)} ms`;
                }
            }
            return '超时';
        } catch (error) {
            console.error(`Failed to ping ${host}:`, error);
            return '失败';
        }
    }

    // 获取已安装应用列表
    static async getInstalledApps() {
        // 1. 获取基础包列表（包名 + 可选的 UID）
        let basePackages = [];

        // 尝试 KSU listPackages
        try {
            const pkgs = await listPackages('all');
            if (pkgs && pkgs.length > 0) {
                basePackages = pkgs.map(p => ({ packageName: p, uid: 0 }));
            }
        } catch (e) {
            console.warn('listPackages failed', e);
        }

        // 如果 KSU 失败或为空，尝试 packages.list
        if (basePackages.length === 0) {
            const systemApps = await this.getPackagesFromSystemList();
            if (systemApps.length > 0) {
                basePackages = systemApps; // 包含 packageName 和 uid
            }
        }

        if (basePackages.length === 0) return [];

        // 2. 丰富应用信息 (Label, Icon, UID)

        // 场景 A: WebUI X 环境
        if (typeof $packageManager !== 'undefined') {
            const apps = await Promise.all(basePackages.map(async pkg => {
                try {
                    // 尝试获取应用信息
                    const info = $packageManager.getApplicationInfo(pkg.packageName, 0, 0);

                    // 获取 Label
                    let label = pkg.packageName;
                    try {
                        // 尝试多种获取 Label 的方式，应对不同版本
                        if (info.getLabel && typeof info.getLabel === 'function') label = info.getLabel();
                        else if (info.loadLabel && typeof info.loadLabel === 'function') label = info.loadLabel($packageManager);
                        else if (info.label) label = info.label;
                        else if (info.toString() !== '[object Object]') label = info.toString();
                    } catch (err) {
                        // 忽略
                    }

                    // 获取 UID (如果 basePackages 里没有)
                    let uid = pkg.uid;
                    if (!uid || uid === 0) {
                        if (info.uid) uid = info.uid;
                        else if (info.applicationInfo && info.applicationInfo.uid) uid = info.applicationInfo.uid;
                    }

                    return {
                        packageName: pkg.packageName,
                        appLabel: label || pkg.packageName,
                        uid: uid,
                        icon: null // 懒加载
                    };
                } catch (e) {
                    // 如果 getApplicationInfo 失败，但我们有 basePackage 信息，还是返回它
                    return {
                        packageName: pkg.packageName,
                        appLabel: pkg.packageName,
                        uid: pkg.uid,
                        icon: null
                    };
                }
            }));

            const validApps = apps.filter(a => a);

            if (validApps.length > 0) {
                // 尝试使用 KSU API 或 packages.list 填充缺失的 UID
                const appsWithMissingUid = validApps.filter(a => !a.uid);
                if (appsWithMissingUid.length > 0) {
                    try {
                        // 策略1: KSU API
                        const ksuApps = await getPackagesInfo(appsWithMissingUid.map(a => a.packageName));
                        const uidMap = {};
                        ksuApps.forEach(a => uidMap[a.packageName] = a.uid);

                        // 策略2: packages.list（如果 KSU API 遗漏或失败）
                        if (Object.keys(uidMap).length < appsWithMissingUid.length) {
                            const systemApps = await this.getPackagesFromSystemList();
                            systemApps.forEach(a => {
                                if (!uidMap[a.packageName]) uidMap[a.packageName] = a.uid;
                            });
                        }

                        appsWithMissingUid.forEach(a => {
                            if (uidMap[a.packageName]) a.uid = uidMap[a.packageName];
                        });
                    } catch (e) {
                        console.warn('Failed to fetch UIDs via KSU API', e);
                        // 回退到 packages.list 获取 UID
                        const systemApps = await this.getPackagesFromSystemList();
                        const uidMap = {};
                        systemApps.forEach(a => uidMap[a.packageName] = a.uid);
                        appsWithMissingUid.forEach(a => {
                            if (uidMap[a.packageName]) a.uid = uidMap[a.packageName];
                        });
                    }
                }
                return validApps;
            }
            console.warn('WebUI X API returned 0 valid apps, falling back to KSU API...');
        }

        // 场景 B: KSU 环境 (或者 WebUI X 彻底失败)
        try {
            // 提取包名列表
            const packageNames = basePackages.map(p => p.packageName);
            const appsInfo = await getPackagesInfo(packageNames);

            return appsInfo.map(app => ({
                packageName: app.packageName,
                appLabel: app.appLabel,
                uid: app.uid,
                icon: `ksu://icon/${app.packageName}`
            }));
        } catch (error) {
            console.error('Failed to get apps via KSU API:', error);
            // Final fallback: 返回 basePackages (可能只有包名和UID)
            return basePackages.map(p => ({
                packageName: p.packageName,
                appLabel: p.packageName,
                uid: p.uid,
                icon: null
            }));
        }
    }

    static async getPackagesFromSystemList() {
        try {
            const content = await this.exec('cat /data/system/packages.list');
            const lines = content.split('\n');
            const apps = [];
            for (const line of lines) {
                const parts = line.split(/\s+/);
                if (parts.length >= 2) {
                    const packageName = parts[0];
                    const uid = parseInt(parts[1]);
                    if (packageName && !isNaN(uid)) {
                        apps.push({
                            packageName,
                            appLabel: packageName, // Fallback label
                            uid,
                            icon: null
                        });
                    }
                }
            }
            return apps;
        } catch (e) {
            console.error('Failed to read packages.list:', e);
            return [];
        }
    }

    static iconCache = new Map();
    static iconLoadQueue = [];
    static activeIconLoads = 0;
    static MAX_CONCURRENT_ICON_LOADS = 4; // 限制并发数，避免旧版 WebUI X 阻塞

    static clearIconLoadQueue() {
        this.iconLoadQueue = [];
    }

    static async loadAppIcon(packageName) {
        if (this.iconCache.has(packageName)) {
            return this.iconCache.get(packageName);
        }

        if (typeof $packageManager === 'undefined') return null;

        return new Promise((resolve) => {
            this.iconLoadQueue.push({ packageName, resolve });
            this.processIconLoadQueue();
        });
    }

    static async processIconLoadQueue() {
        if (this.activeIconLoads >= this.MAX_CONCURRENT_ICON_LOADS || this.iconLoadQueue.length === 0) {
            return;
        }

        this.activeIconLoads++;
        const { packageName, resolve } = this.iconLoadQueue.shift();

        try {
            const base64 = await this._doLoadAppIcon(packageName);
            resolve(base64);
        } catch (e) {
            resolve(null);
        } finally {
            this.activeIconLoads--;
            // 使用 setTimeout 让出主线程，避免连续处理阻塞 UI
            setTimeout(() => this.processIconLoadQueue(), 0);
        }
    }

    static async _doLoadAppIcon(packageName) {
        try {
            const stream = $packageManager.getApplicationIcon(packageName, 0, 0);
            if (!stream) return null;

            const wrapped = await wrapInputStream(stream);
            const buffer = await wrapped.arrayBuffer();

            const base64 = 'data:image/png;base64,' + this.arrayBufferToBase64(buffer);

            this.iconCache.set(packageName, base64);
            return base64;
        } catch (e) {
            return null;
        }
    }

    static arrayBufferToBase64(buffer) {
        let binary = '';
        const bytes = new Uint8Array(buffer);
        const len = bytes.byteLength;
        for (let i = 0; i < len; i++) {
            binary += String.fromCharCode(bytes[i]);
        }
        return btoa(binary);
    }

    // ===================== 代理开关设置 =====================

    // 获取代理开关设置
    static async getProxySettings() {
        try {
            const content = await this.exec(`cat ${this.MODULE_PATH}/config/tproxy.conf`);
            const settings = {};
            const keys = ['PROXY_MOBILE', 'PROXY_WIFI', 'PROXY_HOTSPOT', 'PROXY_USB', 'PROXY_TCP', 'PROXY_UDP', 'PROXY_IPV6'];

            for (const key of keys) {
                const match = content.match(new RegExp(`${key}=(\\d+)`));
                settings[key.toLowerCase()] = match ? match[1] === '1' : false;
            }
            return settings;
        } catch (error) {
            return {
                proxy_mobile: true,
                proxy_wifi: true,
                proxy_hotspot: false,
                proxy_usb: false,
                proxy_tcp: true,
                proxy_udp: true,
                proxy_ipv6: false
            };
        }
    }

    // 设置代理开关
    static async setProxySetting(key, value) {
        const upperKey = key.toUpperCase();
        const numValue = value ? '1' : '0';
        await this.exec(`sed -i 's/${upperKey}=.*/${upperKey}=${numValue}/' ${this.MODULE_PATH}/config/tproxy.conf`);
        return { success: true };
    }

    // 获取代理模式 (0=自动, 1=TPROXY, 2=REDIRECT)
    static async getProxyMode() {
        try {
            const content = await this.exec(`cat ${this.MODULE_PATH}/config/tproxy.conf`);
            const match = content.match(/PROXY_MODE=(\d+)/);
            return match ? parseInt(match[1]) : 0;
        } catch (error) {
            return 0;
        }
    }

    // 设置代理模式
    static async setProxyMode(value) {
        await this.exec(`sed -i 's/^PROXY_MODE=.*/PROXY_MODE=${value}/' ${this.MODULE_PATH}/config/tproxy.conf`);
        return { success: true };
    }

    // ===================== 模块设置 =====================

    // 获取模块设置
    static async getModuleSettings() {
        try {
            const content = await this.exec(`cat ${this.MODULE_PATH}/config/module.conf`);
            const settings = {};

            const autoStartMatch = content.match(/AUTO_START=(\d+)/);
            settings.auto_start = autoStartMatch ? autoStartMatch[1] === '1' : true;

            const oneplusFixMatch = content.match(/ONEPLUS_A16_FIX=(\d+)/);
            settings.oneplus_a16_fix = oneplusFixMatch ? oneplusFixMatch[1] === '1' : true;

            return settings;
        } catch (error) {
            return {
                auto_start: true,
                oneplus_a16_fix: true
            };
        }
    }

    // 设置模块选项
    static async setModuleSetting(key, value) {
        const upperKey = key.toUpperCase();
        const numValue = value ? '1' : '0';
        await this.exec(`sed -i 's/${upperKey}=.*/${upperKey}=${numValue}/' ${this.MODULE_PATH}/config/module.conf`);
        return { success: true };
    }

    // ===================== 路由规则管理 =====================

    // 获取路由规则列表
    static async getRoutingRules() {
        try {
            const output = await this.exec(`cat ${this.MODULE_PATH}/config/routing_rules.json`);
            return JSON.parse(output);
        } catch (error) {
            console.error('获取路由规则失败:', error);
            return [];
        }
    }

    // 保存路由规则列表
    static async saveRoutingRules(rules) {
        try {
            const json = JSON.stringify(rules, null, 4);
            // 使用 base64 编码避免特殊字符问题
            const base64 = btoa(unescape(encodeURIComponent(json)));
            await this.exec(`echo '${base64}' | base64 -d > ${this.MODULE_PATH}/config/routing_rules.json`);
            return { success: true };
        } catch (error) {
            console.error('保存路由规则失败:', error);
            return { success: false, error: error.message };
        }
    }

    // 应用路由规则（在前端生成 routing.json）
    static async applyRoutingRules(rules) {
        try {
            // 构建路由规则数组
            const xrayRules = [];

            for (const rule of rules) {
                if (rule.enabled === false) continue;

                const xrayRule = { type: 'field' };

                // 处理 domain
                if (rule.domain) {
                    xrayRule.domain = rule.domain.split(',').map(d => {
                        d = d.trim();
                        if (d.startsWith('geosite:') || d.startsWith('domain:') || d.startsWith('full:') || d.startsWith('regexp:')) {
                            return d;
                        }
                        return `domain:${d}`;
                    });
                }

                // 处理 ip
                if (rule.ip) {
                    xrayRule.ip = rule.ip.split(',').map(i => i.trim());
                }

                // 处理 port
                if (rule.port) {
                    xrayRule.port = rule.port.trim();
                }

                // 处理 protocol
                if (rule.protocol) {
                    xrayRule.protocol = rule.protocol.split(',').map(p => p.trim());
                }

                // 处理 network
                if (rule.network) {
                    xrayRule.network = rule.network.trim();
                }

                // 设置 outboundTag
                xrayRule.outboundTag = rule.outboundTag || 'proxy';

                xrayRules.push(xrayRule);
            }

            // 添加固定的内部 DNS 规则
            xrayRules.push({
                type: 'field',
                inboundTag: ['domestic-dns'],
                outboundTag: 'direct'
            });
            xrayRules.push({
                type: 'field',
                inboundTag: ['dns-module'],
                outboundTag: 'proxy'
            });

            // 构建完整的路由配置
            const routingConfig = {
                routing: {
                    domainStrategy: 'AsIs',
                    rules: xrayRules
                }
            };

            // 使用 base64 编码写入文件
            const json = JSON.stringify(routingConfig, null, 4);
            const base64 = btoa(unescape(encodeURIComponent(json)));
            await this.exec(`echo '${base64}' | base64 -d > ${this.MODULE_PATH}/config/xray/confdir/03_routing.json`);

            return { success: true };
        } catch (error) {
            console.error('应用路由规则失败:', error);
            return { success: false, error: error.message };
        }
    }

    // ===================== 导出功能 =====================

    // 导出日志到 Download 目录
    static async exportLogs() {
        try {
            const timestamp = new Date().toISOString().replace(/[:-]/g, '').replace('T', '_').slice(0, 15);
            const filename = `netproxy_logs_${timestamp}.tar.gz`;
            const downloadPath = '/storage/emulated/0/Download';
            const outputPath = `${downloadPath}/${filename}`;

            // 确保 Download 目录存在
            await this.exec(`mkdir -p ${downloadPath}`);

            // 使用 tar 压缩 logs 文件夹
            await this.exec(`cd ${this.MODULE_PATH} && tar -czf ${outputPath} logs/`);

            return { success: true, path: outputPath };
        } catch (error) {
            console.error('导出日志失败:', error);
            return { success: false, error: error.message };
        }
    }

    // 导出日志与配置到 Download 目录
    static async exportAll() {
        try {
            const timestamp = new Date().toISOString().replace(/[:-]/g, '').replace('T', '_').slice(0, 15);
            const filename = `netproxy_backup_${timestamp}.tar.gz`;
            const downloadPath = '/storage/emulated/0/Download';
            const outputPath = `${downloadPath}/${filename}`;

            // 确保 Download 目录存在
            await this.exec(`mkdir -p ${downloadPath}`);

            // 使用 tar 同时压缩 logs 和 config 文件夹
            await this.exec(`cd ${this.MODULE_PATH} && tar -czf ${outputPath} logs/ config/`);

            return { success: true, path: outputPath };
        } catch (error) {
            console.error('导出日志与配置失败:', error);
            return { success: false, error: error.message };
        }
    }
}
