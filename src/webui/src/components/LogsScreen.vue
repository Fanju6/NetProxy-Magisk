<script setup lang="ts">
/**
 * @file LogsScreen.vue
 * @description 运行日志子页：在「服务 / 订阅 / 内核」三类日志间切换，定时(2.5s)经 CLI 拉取并增量刷新。
 *   支持卡片视图（解析出时间/级别/标签/出站流向/延迟徽章）与原始等宽视图，并可清空日志。子页形态。
 */
import { ref, onMounted, onUnmounted, nextTick, watch } from 'vue';
import { useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { showToast } from '../utils/ksu';
import { moduleClient, type LogKind } from '../api/moduleClient';
import { useBackDismiss } from '../composables/useBackDismiss';

const router = useRouter();
const { t } = useI18n();

// 深色模式单例：用 matchMedia 监听一次，避免每条日志每渲染重复查询
const darkMql = typeof window !== 'undefined' && window.matchMedia
  ? window.matchMedia('(prefers-color-scheme: dark)')
  : null;
const isDark = ref(darkMql ? darkMql.matches : false);
const onSchemeChange = (e: MediaQueryListEvent) => { isDark.value = e.matches; };

type LogType = LogKind;
const activeLogType = ref<LogType>('service');
const logLineCount = ref(100);
const isLogsLoading = ref(false);
const autoScrollLogs = ref(true);
const subScreenContentRef = ref<HTMLDivElement | null>(null);
let logRefreshTimer: any = null;

// 结构化日志条目：原始行解析后供卡片视图渲染（含预计算的徽章样式，避免每渲染重算）
interface LogItem {
  rawLine: string;
  timestamp: string;
  level: 'DEBUG' | 'INFO' | 'WARN' | 'ERROR' | 'UNKNOWN';
  tag: string;
  message: string;
  outboundFlow?: {
    source: string;
    target: string;
    outbound: string;
  } | null;
  connId?: string | null;
  latency?: string | null;
  cleanTag?: string;
  cleanMessage?: string;
  latencyBadgeStyle?: Record<string, string>;
  outboundBadgeStyle?: Record<string, string>;
  outboundUpper?: string;
}

const logsItems = ref<LogItem[]>([]);
const isCardView = ref(true);
const showMoreMenu = ref(false);

// 手势返回优先关闭"更多"菜单
useBackDismiss(
  () => showMoreMenu.value,
  () => { showMoreMenu.value = false; }
);

// ===================================================================
// 视图切换 / 滚动 / 返回等 UI 动作
// ===================================================================

/** 切换卡片视图 / 原始等宽视图，并收起「更多」菜单。 */
const toggleLogsViewMode = () => {
  isCardView.value = !isCardView.value;
  showMoreMenu.value = false;
};

/** 滚动到日志顶部。 */
const scrollToLogsTop = () => {
  showMoreMenu.value = false;
  nextTick(() => {
    const el = subScreenContentRef.value;
    if (el) el.scrollTop = 0;
  });
};

/** 滚动到日志底部。 */
const scrollToLogsBottom = () => {
  showMoreMenu.value = false;
  nextTick(() => {
    const el = subScreenContentRef.value;
    if (el) el.scrollTop = el.scrollHeight;
  });
};

/** 返回上一页（子页顶栏返回箭头）。 */
const handleBack = () => {
  router.back();
};

// ===================================================================
// 日志解析与格式化
// ===================================================================

/**
 * 将日志级别字符串归一化为枚举级别。
 * @param levelStr  原始级别文本（如 INFO / WARNING / FATAL）
 * @returns 归一化后的级别
 */
const parseLogLevel = (levelStr: string): 'DEBUG' | 'INFO' | 'WARN' | 'ERROR' | 'UNKNOWN' => {
  const l = levelStr.toUpperCase();
  if (l === 'DEBUG') return 'DEBUG';
  if (l === 'INFO') return 'INFO';
  if (l === 'WARN' || l === 'WARNING') return 'WARN';
  if (l === 'ERROR' || l === 'FATAL') return 'ERROR';
  return 'UNKNOWN';
};

/**
 * 无显式级别字段时按关键词（含中文「失败/警告」）猜测级别，默认 INFO。
 * @param line  整行日志
 * @returns 猜测的级别
 */
const guessLogLevel = (line: string): 'DEBUG' | 'INFO' | 'WARN' | 'ERROR' | 'UNKNOWN' => {
  const lower = line.toLowerCase();
  if (lower.includes('error') || lower.includes('fail') || lower.includes('失败') || lower.includes('err')) return 'ERROR';
  if (lower.includes('warn') || lower.includes('warning') || lower.includes('警告')) return 'WARN';
  if (lower.includes('debug')) return 'DEBUG';
  return 'INFO';
};

/**
 * 从内核日志 tag 中提取出站名，形如 `outbound/xxx[tag]` → `tag`。
 * @param tag  日志 tag
 * @returns 出站名（无法解析则原样返回）
 */
const parseOutboundFromTag = (tag: string): string => {
  try {
    const idx = tag.indexOf("outbound/");
    if (idx !== -1) {
      const sub = tag.substring(idx + "outbound/".length);
      const start = sub.indexOf('[');
      const end = sub.indexOf(']');
      if (start !== -1 && end !== -1 && end > start) {
        return sub.substring(start + 1, end);
      }
      return sub;
    }
    return tag;
  } catch {
    return tag;
  }
};

/**
 * 从消息中解析「源 → 目标 [出站]」路由流向。
 * @param message  日志消息体
 * @returns 流向对象 {source,target,outbound}，无匹配则 null
 */
const parseOutboundFlow = (message: string): any => {
  const routingPattern = /routed connection from (\S+) to (\S+) \[(\S+)\]/;
  const match = routingPattern.exec(message);
  if (match) {
    return {
      source: match[1],
      target: match[2],
      outbound: match[3]
    };
  }
  return null;
};

/**
 * 解析「outbound (packet) connection to X」形式的消息，出站名取自 tag。
 * @param message  日志消息体
 * @param tag      日志 tag（用于提取出站名）
 * @returns 流向对象 {source:'Local',target,outbound}，无匹配则 null
 */
const parseDetailOutboundFlow = (message: string, tag: string): any => {
  const outboundConnPattern = /^outbound\s+(?:packet\s+)?connection\s+to\s+(\S+)$/;
  const match = outboundConnPattern.exec(message);
  if (match) {
    const target = match[1];
    const outbound = parseOutboundFromTag(tag);
    return {
      source: "Local",
      target: target,
      outbound: outbound
    };
  }
  return null;
};

/**
 * 把 ISO 时间串裁剪为「MM-DD HH:mm:ss」展示格式。
 * @param isoStr  ISO 时间字符串
 * @returns 展示用时间串（过短则原样返回）
 */
const formatIsoTimestamp = (isoStr: string): string => {
  try {
    if (isoStr.length >= 19) {
      const datePart = isoStr.substring(5, 10);
      const timePart = isoStr.substring(11, 19);
      return `${datePart} ${timePart}`;
    }
    return isoStr;
  } catch {
    return isoStr;
  }
};

/**
 * 把「YYYY-MM-DD HH:mm:ss」裁剪为「MM-DD HH:mm:ss」展示格式。
 * @param dateTimeStr  日期时间字符串
 * @returns 展示用时间串（过短则原样返回）
 */
const formatDateTimeTimestamp = (dateTimeStr: string): string => {
  try {
    if (dateTimeStr.length >= 19) {
      return dateTimeStr.substring(5, 19);
    }
    return dateTimeStr;
  } catch {
    return dateTimeStr;
  }
};

/**
 * 把单行日志解析为结构化 LogItem：按类型套用不同正则（服务/订阅为带方括号级别的简单格式，
 * 内核为多 pattern 兜底），再从 tag/message 中抽取连接 ID 与延迟。全程容错，无匹配则降级原样展示。
 * @param line  原始日志行
 * @param type  日志类型（service / sub / core）
 * @returns 结构化日志条目
 */
const parseLogLine = (line: string, type: LogType): LogItem => {
  let item: LogItem;
  if (type === 'service' || type === 'sub') {
    const serviceLogPattern1 = /^\[([\d\-:\s]+)\]\s+\[([A-Za-z]+)\]\s+(.*)$/;
    const m1 = serviceLogPattern1.exec(line);
    if (m1) {
      item = {
        rawLine: line,
        timestamp: formatDateTimeTimestamp(m1[1].trim()),
        level: parseLogLevel(m1[2]),
        tag: "System",
        message: m1[3]
      };
    } else {
      const serviceLogPattern2 = /^\[([A-Za-z]+)\]:\s+(.*)$/;
      const m2 = serviceLogPattern2.exec(line);
      if (m2) {
        item = {
          rawLine: line,
          timestamp: "",
          level: parseLogLevel(m2[1]),
          tag: "System",
          message: m2[2]
        };
      } else {
        item = {
          rawLine: line,
          timestamp: "",
          level: guessLogLevel(line),
          tag: "System",
          message: line
        };
      }
    }
  } else {
    // core：内核(sing-box)日志，格式更复杂，下面多种 pattern 依次兜底
    const kernelLogPattern3 = /^([+-]\d{4})\s+([\d\-:\s]+)\s+([A-Z]+)\s+([^:]+):\s+(.*)$/;
    const m3 = kernelLogPattern3.exec(line);
    if (m3) {
      const timestamp = formatDateTimeTimestamp(m3[2].trim());
      const level = parseLogLevel(m3[3]);
      const tag = m3[4];
      const message = m3[5];
      const flow = parseOutboundFlow(message) || parseDetailOutboundFlow(message, tag);
      item = {
        rawLine: line,
        timestamp,
        level,
        tag,
        message,
        outboundFlow: flow
      };
    } else {
      const kernelLogPattern1 = /^([\d\-T:.Z+]+)\s+([A-Z]+)\s+([^:]+):\s+(.*)$/;
      const m1 = kernelLogPattern1.exec(line);
      if (m1) {
        const timestamp = formatIsoTimestamp(m1[1]);
        const level = parseLogLevel(m1[2]);
        const tag = m1[3];
        const message = m1[4];
        const flow = parseOutboundFlow(message) || parseDetailOutboundFlow(message, tag);
        item = {
          rawLine: line,
          timestamp,
          level,
          tag,
          message,
          outboundFlow: flow
        };
      } else {
        const kernelLogPattern2 = /^([A-Z]+)\[\d+\]\s+(?:\[\d+\]\s+)?([^:]+):\s+(.*)$/;
        const m2 = kernelLogPattern2.exec(line);
        if (m2) {
          const level = parseLogLevel(m2[1]);
          const tag = m2[2];
          const message = m2[3];
          const flow = parseOutboundFlow(message) || parseDetailOutboundFlow(message, tag);
          item = {
            rawLine: line,
            timestamp: "",
            level,
            tag,
            message,
            outboundFlow: flow
          };
        } else {
          item = {
            rawLine: line,
            timestamp: "",
            level: 'UNKNOWN',
            tag: "Kernel",
            message: line
          };
        }
      }
    }
  }

  let connId: string | null = null;
  let latency: string | null = null;
  let cleanTag = item.tag;
  let cleanMessage = item.message;

  if (type === 'core') {
    const rx = /^\[(\d+)(?:\s+([^\]\s]+))?\s*\]\s*(.*)$/;
    const tagMatch = rx.exec(item.tag);
    if (tagMatch) {
      connId = tagMatch[1];
      latency = tagMatch[2] || null;
      cleanTag = tagMatch[3];
    } else {
      const msgMatch = rx.exec(item.message);
      if (msgMatch) {
        connId = msgMatch[1];
        latency = msgMatch[2] || null;
        cleanMessage = msgMatch[3];
      }
    }
  }

  item.connId = connId;
  item.latency = latency;
  item.cleanTag = cleanTag;
  item.cleanMessage = cleanMessage;

  return item;
};

/**
 * 把延迟字符串解析为毫秒数（支持 ms / s 后缀）。
 * @param duration  延迟文本（如 "123ms" / "1.2s"）
 * @returns 毫秒数（无法解析则 0）
 */
const parseLatencyMs = (duration: string): number => {
  try {
    const numberPart = duration.replace(/[^0-9.]/g, '');
    const d = parseFloat(numberPart) || 0;
    if (duration.toLowerCase().endsWith('ms')) {
      return Math.round(d);
    } else if (duration.toLowerCase().endsWith('s')) {
      return Math.round(d * 1000);
    }
    return 0;
  } catch {
    return 0;
  }
};

// ===================================================================
// 徽章样式：按延迟高低 / 出站类型着色（区分明暗主题）
// ===================================================================

/**
 * 按延迟高低返回徽章配色（<150ms 绿 / <500ms 橙 / 否则红），区分明暗主题。
 * @param latency  延迟文本，空则返回空样式
 * @returns 内联样式对象
 */
const getLatencyBadgeStyle = (latency: string | null): Record<string, string> => {
  if (!latency) return {};
  const ms = parseLatencyMs(latency);
  const dark = isDark.value;
  if (ms < 150) {
    return {
      backgroundColor: dark ? 'rgba(27, 94, 32, 0.3)' : '#e8f5e9',
      color: dark ? '#81c784' : '#2e7d32'
    };
  } else if (ms < 500) {
    return {
      backgroundColor: dark ? 'rgba(230, 81, 0, 0.3)' : '#fff3e0',
      color: dark ? '#ffb74d' : '#e65100'
    };
  } else {
    return {
      backgroundColor: dark ? 'rgba(183, 28, 28, 0.3)' : 'rgba(255, 235, 238, 0.9)',
      color: dark ? '#e57373' : '#c62828'
    };
  }
};

/**
 * 按出站类型返回徽章配色（direct 绿 / block·reject 红 / 其余主色），区分明暗主题。
 * @param outbound  出站名
 * @returns 内联样式对象
 */
const getOutboundBadgeStyle = (outbound: string): Record<string, string> => {
  const dark = isDark.value;
  const o = outbound.toLowerCase();
  if (o === 'direct') {
    return {
      backgroundColor: dark ? 'rgba(27, 94, 32, 0.3)' : '#e8f5e9',
      color: dark ? '#81c784' : '#2e7d32'
    };
  } else if (o === 'block' || o === 'reject') {
    return {
      backgroundColor: dark ? 'rgba(183, 28, 28, 0.3)' : 'rgba(255, 235, 238, 0.9)',
      color: dark ? '#e57373' : '#c62828'
    };
  } else {
    return {
      backgroundColor: dark ? 'rgba(103, 80, 164, 0.2)' : 'var(--md-sys-color-primary-container)',
      color: dark ? 'var(--md-sys-color-primary)' : 'var(--md-sys-color-on-primary-container)'
    };
  }
};

/** 把徽章样式、出站大写名等展示属性预计算进 LogItem，模板直接读，避免每渲染重算。 */
const decorateLogItem = (item: LogItem): LogItem => {
  item.latencyBadgeStyle = getLatencyBadgeStyle(item.latency ?? null);
  if (item.outboundFlow) {
    item.outboundBadgeStyle = getOutboundBadgeStyle(item.outboundFlow.outbound);
    item.outboundUpper = item.outboundFlow.outbound.toUpperCase();
  }
  return item;
};

let lastRawLogs = '';   // 上次原始输出，用于跳过未变化的重解析

// ===================================================================
// 日志加载 / 清空 / 定时轮询
// ===================================================================

/**
 * 拉取当前类型日志并解析渲染：真机经 CLI、mock 用内置假数据。原始输出未变化则跳过重解析，
 * 避免静默期每 2.5s 无谓 churn；开启自动滚动时刷新后滚到底部。
 * @param showLoader  是否先清空并显示加载指示（切换 tab 时用）
 */
const loadLogs = async (showLoader = false) => {
  if (showLoader) {
    isLogsLoading.value = true;
    logsItems.value = []; // 立即清空以显示加载指示，避免短暂看到其他 tab 的旧日志
    lastRawLogs = ' '; // 强制本次重新解析
  }
  try {
    const raw = await moduleClient.readLogs(activeLogType.value, logLineCount.value);

    // 原始输出未变化则跳过重解析/重渲染，避免静默期每 2.5s churn
    if (raw === lastRawLogs) {
      return;
    }
    lastRawLogs = raw;

    const lines = raw.replace(/\r\n/g, '\n').split('\n').filter(Boolean);
    logsItems.value = lines.map(line => decorateLogItem(parseLogLine(line, activeLogType.value)));

    if (autoScrollLogs.value) {
      nextTick(() => {
        const el = subScreenContentRef.value;
        if (el) el.scrollTop = el.scrollHeight;
      });
    }
  } catch (err: any) {
    console.error('Failed to load logs:', err);
    logsItems.value = []; // 命令失败（如日志文件尚不存在）则清空当前日志
    lastRawLogs = '';
  } finally {
    isLogsLoading.value = false;
  }
};

/** 清空当前类型的日志文件（真机经 CLI），并清空界面、收起菜单。 */
const handleClearLogs = async () => {
  try {
    await moduleClient.clearLogs(activeLogType.value);
    showToast(t('logs.cleared'));
    logsItems.value = [];
  } catch (err: any) {
    showToast(t('logs.clearFailed', { msg: err.message || err }));
  }
  showMoreMenu.value = false;
};



/**
 * 日志类型 tab 切换回调：切换类型并以加载态重新拉取。
 * @param e  md-tabs 的 change 事件
 */
const handleLogTabChange = (e: Event) => {
  const tabs = e.target as any;
  const index = tabs.activeTabIndex;
  const types: LogType[] = ['service', 'sub', 'core'];
  const newType = types[index] || 'service';
  if (activeLogType.value !== newType) {
    activeLogType.value = newType;
    loadLogs(true);
  }
};

/** 启动日志轮询：先全量加载一次，随后每 2.5s 增量刷新（先停旧定时器，保证幂等）。 */
const startLogsSync = () => {
  stopLogsSync();
  loadLogs(true);
  logRefreshTimer = setInterval(() => {
    loadLogs(false);
  }, 2500);
};

/** 停止日志轮询定时器。 */
const stopLogsSync = () => {
  if (logRefreshTimer) {
    clearInterval(logRefreshTimer);
    logRefreshTimer = null;
  }
};

onMounted(() => {
  // 订阅深色模式变化：变化时重算现有日志的徽章样式
  if (darkMql) {
    darkMql.addEventListener('change', onSchemeChange);
  }
  startLogsSync();
});

onUnmounted(() => {
  stopLogsSync();
  if (darkMql) {
    darkMql.removeEventListener('change', onSchemeChange);
  }
});

// 深色模式切换时，重新装饰现有日志条目 (无需重新拉取/解析)
watch(isDark, () => {
  logsItems.value = logsItems.value.map(item => decorateLogItem(item));
});
</script>

<template>
  <Teleport to="body">
    <div class="sub-screen-overlay scroll-container">
      <header class="sub-top-bar">
        <div class="sub-top-bar-left">
          <md-icon-button @click="handleBack" class="sub-back-btn">
            <md-icon>
              <svg viewBox="0 0 24 24">
                <path d="M20 11H7.83l5.59-5.59L12 4l-8 8 8 8 1.41-1.41L7.83 13H20v-2z" fill="currentColor" />
              </svg>
            </md-icon>
          </md-icon-button>
          <h1 class="sub-screen-title">{{ t('logs.title') }}</h1>
        </div>

        <div class="sub-top-bar-right">
          <div class="more-menu-wrapper">
            <md-icon-button @click="showMoreMenu = !showMoreMenu" :title="t('logs.moreOptions')">
              <md-icon>
                <svg viewBox="0 0 24 24">
                  <path d="M12 8c1.1 0 2-.9 2-2s-.9-2-2-2-2 .9-2 2 .9 2 2 2zm0 2c-1.1 0-2 .9-2 2s.9 2 2 2 2-.9 2-2-.9-2-2-2zm0 6c-1.1 0-2 .9-2 2s.9 2 2 2 2-.9 2-2-.9-2-2-2z" fill="currentColor"/>
                </svg>
              </md-icon>
            </md-icon-button>
            <div class="more-menu-dropdown-box" v-if="showMoreMenu">
              <div class="more-menu-dropdown-scrim" @click="showMoreMenu = false"></div>
              <div class="more-menu-dropdown">
                <div class="menu-item" @click="toggleLogsViewMode">
                  {{ isCardView ? t('logs.rawView') : t('logs.cardView') }}
                </div>
                <div class="menu-divider"></div>
                <div class="menu-item" @click="scrollToLogsTop">{{ t('logs.scrollToTop') }}</div>
                <div class="menu-item" @click="scrollToLogsBottom">{{ t('logs.scrollToBottom') }}</div>
                <div class="menu-divider"></div>
                <div class="menu-item danger" @click="handleClearLogs">{{ t('logs.clearNow') }}</div>
              </div>
            </div>
          </div>
        </div>
      </header>

      <!-- 固定的日志类型切换 tab 栏 -->
      <div class="sub-fixed-header-extra">
        <md-tabs 
          :active-tab-index="activeLogType === 'service' ? 0 : activeLogType === 'sub' ? 1 : 2" 
          @change="handleLogTabChange"
          style="width: 100%; --md-primary-tab-container-color: transparent;">
          <md-primary-tab>{{ t('logs.tabService') }}</md-primary-tab>
          <md-primary-tab>{{ t('logs.tabSub') }}</md-primary-tab>
          <md-primary-tab>{{ t('logs.tabCore') }}</md-primary-tab>
        </md-tabs>
      </div>

      <div class="sub-screen-content" ref="subScreenContentRef">
        <div class="logs-page-container">
          <div v-if="isLogsLoading && logsItems.length === 0" class="log-loading-box">
            <md-circular-progress indeterminate style="--md-circular-progress-size: 24px;"></md-circular-progress>
            <span>{{ t('logs.fetching') }}</span>
          </div>
          <div v-else-if="logsItems.length === 0" class="log-empty-box">
            <span v-if="activeLogType === 'service'">{{ t('logs.emptyService') }}</span>
            <span v-else-if="activeLogType === 'sub'">{{ t('logs.emptySub') }}</span>
            <span v-else-if="activeLogType === 'core'">{{ t('logs.emptyCore') }}</span>
            <span v-else>{{ t('logs.emptyDefault') }}</span>
          </div>
          
          <!-- 卡片视图：结构化展示 -->
          <div v-else-if="isCardView" class="logs-cards-container">
            <div v-for="(item, index) in logsItems" :key="index" class="log-card">
              <div class="log-card-header">
                <div class="log-header-left">
                  <span :class="['log-level-badge', 'badge-' + item.level.toLowerCase()]">{{ item.level }}</span>
                  <span class="log-tag">{{ item.cleanTag }}</span>
                </div>
                <span class="log-time" v-if="item.timestamp">{{ item.timestamp }}</span>
              </div>

              <div class="log-badges-row" v-if="item.connId">
                <span class="conn-badge">#{{ item.connId }}</span>
                <span v-if="item.latency" :style="item.latencyBadgeStyle" class="latency-badge">
                  {{ item.latency }}
                </span>
              </div>

              <div class="outbound-flow-card" v-if="activeLogType === 'core' && item.outboundFlow">
                <div class="flow-row">
                  <div class="flow-col">
                    <span class="flow-label">Source</span>
                    <span class="flow-value">{{ item.outboundFlow.source }}</span>
                  </div>
                  <svg class="flow-arrow-icon" viewBox="0 0 24 24">
                    <path d="M12 4l-1.41 1.41L16.17 11H4v2h12.17l-5.58 5.59L12 20l8-8z" fill="currentColor"/>
                  </svg>
                  <div class="flow-col right">
                    <span class="flow-label">Destination</span>
                    <span class="flow-value">{{ item.outboundFlow.target }}</span>
                  </div>
                </div>
                <div class="flow-outbound-row">
                  <span :style="item.outboundBadgeStyle" class="outbound-badge">
                    {{ item.outboundUpper }}
                  </span>
                </div>
              </div>

              <div class="log-message" v-else>
                {{ item.cleanMessage }}
              </div>
            </div>
          </div>

          <!-- 原始等宽视图 -->
          <div v-else class="raw-logs-card">
            <div class="raw-logs-content">
              <div v-for="(item, index) in logsItems" :key="index" class="raw-log-line">
                {{ item.rawLine }}
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
/* 子页覆盖层与顶栏（与原 SettingsScreen 一致的层叠样式） */
.sub-screen-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  width: 100vw;
  height: 100vh;
  background-color: var(--md-sys-color-background);
  z-index: 9999;
  display: flex;
  flex-direction: column;
  box-sizing: border-box;
  overflow: hidden;
  animation: slideUp 0.3s cubic-bezier(0.2, 0.8, 0.2, 1) forwards;
}

@keyframes slideUp {
  from {
    opacity: 0;
    transform: translateY(30px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.sub-top-bar {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: calc(64px + var(--top-inset));
  padding-top: var(--top-inset);
  padding-left: 16px;
  padding-right: 16px;
  color: var(--md-sys-color-on-background);
  background-color: var(--md-sys-color-background);
  border-bottom: 1px solid var(--md-sys-color-outline-variant);
  box-sizing: border-box;
  width: 100%;
}

.sub-top-bar-left {
  display: flex;
  align-items: center;
  gap: 12px;
}

.sub-back-btn {
  --md-icon-button-container-width: 40px;
  --md-icon-button-container-height: 40px;
}

.sub-screen-title {
  font-size: 20px;
  font-weight: 550;
  margin: 0;
}

.sub-fixed-header-extra {
  flex-shrink: 0;
  border-bottom: 1px solid var(--md-sys-color-outline-variant);
  background-color: var(--md-sys-color-surface-container);
  width: 100%;
}

.sub-screen-content {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  overflow-x: hidden;
  padding: 16px;
  padding-bottom: calc(24px + var(--bottom-inset));
  box-sizing: border-box;
  width: 100%;
}

.logs-page-container {
  display: flex;
  flex-direction: column;
  width: 100%;
  height: 100%;
  max-width: 800px;
  margin: 0 auto;
  gap: 12px;
}

.log-loading-box, .log-empty-box {
  padding: 60px 0;
  display: flex;
  flex-direction: column;
  justify-content: center;
  align-items: center;
  gap: 12px;
  color: var(--md-sys-color-on-surface-variant);
  font-size: 14px;
}

.logs-cards-container {
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: 100%;
}

.log-card {
  background-color: var(--md-sys-color-surface-container);
  border: 1px solid var(--md-sys-color-outline-variant);
  border-radius: var(--radius-sm);
  padding: 12px;
  display: flex;
  flex-direction: column;
  width: 100%;
  box-sizing: border-box;
}

.log-card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  width: 100%;
  margin-bottom: 6px;
}

.log-header-left {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.log-level-badge {
  font-size: 10px;
  font-weight: bold;
  padding: 2px 6px;
  border-radius: var(--radius-xs);
  flex-shrink: 0;
}

.badge-info {
  background-color: rgba(13, 71, 161, 0.15);
  color: #1976d2;
}

.badge-warn {
  background-color: rgba(230, 81, 0, 0.15);
  color: #e65100;
}

.badge-error {
  background-color: rgba(183, 28, 28, 0.15);
  color: #c62828;
}

.badge-debug {
  background-color: rgba(27, 94, 32, 0.15);
  color: #2e7d32;
}

.badge-unknown {
  background-color: rgba(55, 71, 79, 0.15);
  color: #616161;
}

@media (prefers-color-scheme: dark) {
  .badge-info {
    background-color: rgba(13, 71, 161, 0.3);
    color: #64b5f6;
  }
  .badge-warn {
    background-color: rgba(230, 81, 0, 0.3);
    color: #ffb74d;
  }
  .badge-error {
    background-color: rgba(183, 28, 28, 0.3);
    color: #e57373;
  }
  .badge-debug {
    background-color: rgba(27, 94, 32, 0.3);
    color: #81c784;
  }
  .badge-unknown {
    background-color: rgba(55, 71, 79, 0.3);
    color: #b0bec5;
  }
}

.log-tag {
  font-size: 12px;
  font-weight: bold;
  color: var(--md-sys-color-on-surface-variant);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.log-time {
  font-size: 11px;
  color: var(--md-sys-color-on-surface-variant);
  flex-shrink: 0;
}

.log-badges-row {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 4px;
  margin-bottom: 6px;
}

.conn-badge {
  font-size: 10px;
  font-weight: bold;
  padding: 2px 6px;
  border-radius: var(--radius-xs);
  background-color: #f3e5f5;
  color: #7b1fa2;
}

@media (prefers-color-scheme: dark) {
  .conn-badge {
    background-color: rgba(74, 20, 140, 0.3);
    color: #ba68c8;
  }
}

.latency-badge {
  font-size: 10px;
  font-weight: bold;
  padding: 2px 6px;
  border-radius: var(--radius-xs);
}

.log-message {
  font-size: 13px;
  color: var(--md-sys-color-on-surface);
  line-height: 1.5;
  word-break: break-all;
  white-space: pre-wrap;
}

/* 出站流向卡片 */
.outbound-flow-card {
  background-color: var(--md-sys-color-surface-container-low);
  border-radius: var(--radius-sm);
  padding: 10px;
  display: flex;
  flex-direction: column;
  width: 100%;
  box-sizing: border-box;
  margin-top: 4px;
}

.flow-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  width: 100%;
}

.flow-col {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-width: 0;
}

.flow-col.right {
  align-items: flex-end;
  text-align: right;
}

.flow-label {
  font-size: 10px;
  color: var(--md-sys-color-on-surface-variant);
}

.flow-value {
  font-size: 13px;
  font-weight: 500;
  color: var(--md-sys-color-on-surface);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  width: 100%;
}

.flow-arrow-icon {
  width: 16px;
  height: 16px;
  color: var(--md-sys-color-on-surface-variant);
  flex-shrink: 0;
  margin: 0 8px;
}

.flow-outbound-row {
  display: flex;
  justify-content: flex-end;
  margin-top: 6px;
}

.outbound-badge {
  font-size: 10px;
  font-weight: bold;
  padding: 2px 6px;
  border-radius: var(--radius-xs);
}

/* 原始等宽日志样式 */
.raw-logs-card {
  background-color: var(--md-sys-color-surface-container);
  border: 1px solid var(--md-sys-color-outline-variant);
  border-radius: var(--radius-lg);
  padding: 12px;
  width: 100%;
  box-sizing: border-box;
}

.raw-logs-content {
  font-family: var(--md-ref-typeface-mono);
  font-size: 11px;
  line-height: 1.5;
  color: var(--md-sys-color-on-surface);
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.raw-log-line {
  white-space: pre-wrap;
  word-break: break-all;
}

/* 「更多」下拉菜单样式 */
.more-menu-wrapper {
  position: relative;
}

.more-menu-dropdown-box {
  position: absolute;
  top: 0;
  right: 0;
  width: 0;
  height: 0;
  overflow: visible;
  pointer-events: none;
}

.more-menu-dropdown-scrim {
  position: fixed;
  top: 0;
  left: 0;
  width: 100vw;
  height: 100vh;
  z-index: 10000;
  background: transparent;
  pointer-events: auto;
}

.more-menu-dropdown {
  position: absolute;
  top: 42px;
  right: 0;
  width: 160px;
  background-color: var(--md-sys-color-surface-container-high);
  border: 1px solid var(--md-sys-color-outline-variant);
  border-radius: var(--radius-sm);
  box-shadow: var(--md-sys-elevation-2);
  z-index: 10001;
  display: flex;
  flex-direction: column;
  padding: 4px 0;
  pointer-events: auto;
}

.menu-item {
  padding: 10px 16px;
  font-size: 14px;
  color: var(--md-sys-color-on-surface);
  cursor: pointer;
  text-align: left;
  transition: background-color 0.15s;
}

.menu-item:hover {
  background-color: var(--md-sys-color-surface-container-highest);
}

.menu-item.danger {
  color: var(--md-sys-color-error);
}

.menu-divider {
  height: 1px;
  background-color: var(--md-sys-color-outline-variant);
  margin: 4px 0;
  opacity: 0.6;
}
</style>
