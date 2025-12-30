import { KSUService } from '../services/ksu-service.js';
import { toast } from '../utils/toast.js';

/**
 * 日志页面管理器
 */
export class LogsPageManager {
    constructor(ui) {
        this.ui = ui;
        this.setupEventListeners();
    }

    setupEventListeners() {
        // 导出日志按钮
        const exportLogsBtn = document.getElementById('export-logs-btn');
        if (exportLogsBtn) {
            exportLogsBtn.addEventListener('click', () => this.exportLogs());
        }

        // 导出日志与配置按钮
        const exportAllBtn = document.getElementById('export-all-btn');
        if (exportAllBtn) {
            exportAllBtn.addEventListener('click', () => this.exportAll());
        }
    }

    async update() {
        await this.loadServiceLog();
        await this.loadXrayLog();
        await this.loadTproxyLog();
        await this.loadUpdateLog();
    }

    async loadServiceLog() {
        try {
            const log = await KSUService.getServiceLog();
            document.getElementById('service-log').textContent = log;
        } catch (error) {
            document.getElementById('service-log').textContent = '加载失败';
        }
    }

    async loadXrayLog() {
        try {
            const log = await KSUService.getXrayLog();
            document.getElementById('xray-log').textContent = log;
        } catch (error) {
            document.getElementById('xray-log').textContent = '加载失败';
        }
    }

    async loadTproxyLog() {
        try {
            const log = await KSUService.getTproxyLog();
            document.getElementById('tproxy-log').textContent = log;
        } catch (error) {
            document.getElementById('tproxy-log').textContent = '加载失败';
        }
    }

    async loadUpdateLog() {
        try {
            const log = await KSUService.getUpdateLog();
            document.getElementById('update-log').textContent = log;
        } catch (error) {
            document.getElementById('update-log').textContent = '加载失败';
        }
    }

    async exportLogs() {
        const btn = document.getElementById('export-logs-btn');
        if (btn) btn.loading = true;

        try {
            const result = await KSUService.exportLogs();
            if (result.success) {
                toast(`日志已保存到: ${result.path}`);
            } else {
                toast('保存日志失败: ' + (result.error || '未知错误'));
            }
        } catch (error) {
            console.error('导出日志失败:', error);
            toast('保存日志失败');
        } finally {
            if (btn) btn.loading = false;
        }
    }

    async exportAll() {
        const btn = document.getElementById('export-all-btn');
        if (btn) btn.loading = true;

        try {
            const result = await KSUService.exportAll();
            if (result.success) {
                toast(`日志与配置已保存到: ${result.path}`);
            } else {
                toast('保存失败: ' + (result.error || '未知错误'));
            }
        } catch (error) {
            console.error('导出日志与配置失败:', error);
            toast('保存失败');
        } finally {
            if (btn) btn.loading = false;
        }
    }
}

