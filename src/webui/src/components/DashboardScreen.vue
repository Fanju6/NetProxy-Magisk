<script setup lang="ts">
/**
 * @file DashboardScreen.vue
 * @description 仪表盘页：服务开关、实时流量曲线(canvas 插值动画)、CPU/内存/运行时长、内网 IP、
 *   出站模式切换。运行指标由单次 shell 读 /proc 解析，流量/节点/模式经 Clash API 拉取；
 *   切走时暂停轮询。非真机环境走 mock 数据。
 */
import { ref, computed, onMounted, onUnmounted, onActivated, onDeactivated, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { runCli, readFileContent, isKsuEnv, showToast, execAsync } from '../utils/ksu';
import { useBackDismiss } from '../composables/useBackDismiss';

const emit = defineEmits(['navigate']);
const { t } = useI18n();

// 仪表盘展示数据（统计轮询时就地更新）
const stats = ref({
  isRunning: false,
  mode: 1, // 0=直连 1=规则 2=全局 3=去广告
  isWifiDirect: false,
  currentNode: '',
  downloadSpeed: 0,
  uploadSpeed: 0,
  downloadTotal: 0,
  uploadTotal: 0,
  cpuUsage: 0,
  memUsage: 0,
  uptime: '服务未开启',
  internalIp: '--'
});

const isPending = ref(false);
const speedCanvas = ref<HTMLCanvasElement | null>(null);
const dlHistory = ref<number[]>(Array(40).fill(0));
const ulHistory = ref<number[]>(Array(40).fill(0));
const showModeDialog = ref(false);

// 手势返回优先关闭出站模式弹窗（而非退出 webui）
useBackDismiss(
  () => showModeDialog.value,
  () => { showModeDialog.value = false; }
);

interface Point {
  x: number;
  y: number;
}

let prevDownloadPoints: Point[] = Array(40).fill(0).map((_, i) => ({ x: i / 39, y: 0 }));
let currentDownloadPoints: Point[] = Array(40).fill(0).map((_, i) => ({ x: i / 39, y: 0 }));
let prevUploadPoints: Point[] = Array(40).fill(0).map((_, i) => ({ x: i / 39, y: 0 }));
let currentUploadPoints: Point[] = Array(40).fill(0).map((_, i) => ({ x: i / 39, y: 0 }));

let chartAnimationStartTime = 0;
const chartAnimationDuration = 300; // ms

// Canvas 渐变缓存：仅在画布高度变化时重建，避免每帧 createLinearGradient
let cachedGradH = -1;
let dlFillGrad: CanvasGradient | null = null;
let ulFillGrad: CanvasGradient | null = null;

/** 重置上一帧/当前帧曲线插值点为全 0（停服时清空图表）。 */
const resetPoints = () => {
  prevDownloadPoints = Array(40).fill(0).map((_, i) => ({ x: i / 39, y: 0 }));
  currentDownloadPoints = Array(40).fill(0).map((_, i) => ({ x: i / 39, y: 0 }));
  prevUploadPoints = Array(40).fill(0).map((_, i) => ({ x: i / 39, y: 0 }));
  currentUploadPoints = Array(40).fill(0).map((_, i) => ({ x: i / 39, y: 0 }));
};

const servicePid = ref('');
const systemTotalMem = ref(8 * 1024 * 1024 * 1024);
const cpuCores = ref(8);
const apiHost = ref('127.0.0.1:9999');
const apiSecret = ref('singbox');

let lastProcessTicks = 0;
let lastTotalCpuTicks = 0;
let lastDlTotal = 0;
let lastUlTotal = 0;
let lastTime = 0;

let statsTimer: number | null = null;
let drawRequestFrame: number | null = null;

const modes = computed(() => [
  t('dashboard.modeDirect'),
  t('dashboard.modeRule'),
  t('dashboard.modeGlobal'),
  t('dashboard.modeAllowAds')
]);
const modeDescriptions = computed(() => [
  t('dashboard.modeDirectDesc'),
  t('dashboard.modeRuleDesc'),
  t('dashboard.modeGlobalDesc'),
  t('dashboard.modeAllowAdsDesc')
]);
const currentModeLabel = computed(() => (
  stats.value.isWifiDirect ? t('dashboard.modeWifiDirect') : modes.value[stats.value.mode]
));
const currentModeDescription = computed(() => (
  stats.value.isWifiDirect ? t('dashboard.wifiDirectAutoHint') : t('dashboard.outboundModeDesc')
));

/**
 * 把字节数格式化为带单位的可读字符串（B/KB/MB/GB/TB）。
 * @param bytes  字节数
 * @returns 可读字符串
 */
const formatBytes = (bytes: number): string => {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
};

/**
 * 把速率格式化为「可读字节/s」。
 * @param bytesPerSec  每秒字节数
 * @returns 速率字符串
 */
const formatSpeed = (bytesPerSec: number): string => {
  return formatBytes(bytesPerSec) + '/s';
};

// 展示值 computed 化：依赖不变时不重算，避免模板每次渲染调用函数
const downloadSpeedText = computed(() => formatSpeed(stats.value.downloadSpeed));
const uploadSpeedText = computed(() => formatSpeed(stats.value.uploadSpeed));
const downloadTotalText = computed(() => formatBytes(stats.value.downloadTotal));
const uploadTotalText = computed(() => formatBytes(stats.value.uploadTotal));
const cpuText = computed(() => stats.value.cpuUsage.toFixed(1));
const memText = computed(() => stats.value.memUsage.toFixed(1));

/**
 * 出站模式名 → 数字（direct=0 / global=2 / allowads=3 / 其余=1 规则）。
 * @param modeStr  模式名
 * @returns 模式数字
 */
const modeStringToNumber = (modeStr: string): number => {
  const m = modeStr.toLowerCase();
  if (m === 'direct') return 0;
  if (m === 'global') return 2;
  if (m === 'allowads') return 3;
  return 1; // 默认规则模式
};

/**
 * 出站模式数字 → 名（用于下发 CLI）。
 * @param modeNum  模式数字
 * @returns 模式名（rule/direct/global/AllowAds）
 */
const modeNumberToString = (modeNum: number): string => {
  if (modeNum === 0) return 'direct';
  if (modeNum === 2) return 'global';
  if (modeNum === 3) return 'AllowAds';
  return 'rule';
};

// ===================================================================
// 数据加载：硬件常量 / API 配置 / 内网 IP / 流量速率
// ===================================================================

/** 读取总内存 (/proc/meminfo) 与 CPU 核心数 (/proc/stat)，供后续占用率换算。 */
const loadHardwareConstants = async () => {
  if (!isKsuEnv()) return;
  try {
    const meminfo = await readFileContent('/proc/meminfo');
    const match = /MemTotal:\s*(\d+)\s*kB/.exec(meminfo);
    if (match) {
      systemTotalMem.value = parseInt(match[1]) * 1024;
    }
  } catch (e) {
    console.warn('Failed to read total meminfo:', e);
  }

  try {
    const procStat = await readFileContent('/proc/stat');
    const lines = procStat.split('\n');
    const cores = lines.filter(line => /^cpu\d+/.test(line)).length;
    if (cores > 0) {
      cpuCores.value = cores;
    }
  } catch (e) {
    console.warn('Failed to read cpu cores:', e);
  }
};

/** 从 02_experimental.json 读取 Clash API 的 external_controller/secret（0.0.0.0 改写为 127.0.0.1）。 */
const loadApiConfig = async () => {
  if (!isKsuEnv()) return;
  try {
    const content = await readFileContent('/data/adb/modules/netproxy/config/singbox/confdir/02_experimental.json');
    const jsonObj = JSON.parse(content);
    const clashApi = jsonObj?.experimental?.clash_api;
    let host = clashApi?.external_controller || '127.0.0.1:9999';
    const secret = clashApi?.secret || 'singbox';
    if (host.startsWith('0.0.0.0:')) {
      host = '127.0.0.1:' + host.substring(8);
    } else if (host === '0.0.0.0') {
      host = '127.0.0.1:9999';
    }
    apiHost.value = host;
    apiSecret.value = secret;
  } catch (e) {
    console.warn('Failed to read API config:', e);
  }
};

/** 通过 `ip route get` 探测内网出口 IP 与网卡名，写入 stats.internalIp。 */
const loadInternalIp = async () => {
  if (!isKsuEnv()) {
    stats.value.internalIp = '10.0.0.2';
    return;
  }
  try {
    const res = await execAsync('ip route get 1.1.1.1');
    if (res.errno === 0) {
      const match = /src\s+(\S+)/.exec(res.stdout);
      const devMatch = /dev\s+(\S+)/.exec(res.stdout);
      if (match) {
        const ip = match[1];
        const dev = devMatch ? devMatch[1] : '';
        stats.value.internalIp = dev ? `${ip} ${dev}` : ip;
      }
    }
  } catch (e) {
    console.warn('Failed to get internal IP:', e);
  }
};

/**
 * 用本次累计上下行总量与上次的差值计算瞬时速率，并刷新累计量。
 * @param dlTotal  下行累计字节
 * @param ulTotal  上行累计字节
 */
const updateTrafficSpeed = (dlTotal: number, ulTotal: number) => {
  const now = Date.now();
  if (lastTime > 0) {
    const elapsed = (now - lastTime) / 1000;
    if (elapsed > 0) {
      const dlSpeed = Math.max(0, (dlTotal - lastDlTotal) / elapsed);
      const ulSpeed = Math.max(0, (ulTotal - lastUlTotal) / elapsed);
      stats.value.downloadSpeed = dlSpeed;
      stats.value.uploadSpeed = ulSpeed;
    }
  }
  lastDlTotal = dlTotal;
  lastUlTotal = ulTotal;
  lastTime = now;
  
  stats.value.downloadTotal = dlTotal;
  stats.value.uploadTotal = ulTotal;
};

// ===================================================================
// Clash API 访问与统计
// ===================================================================

/**
 * 请求 Clash API：优先原生 fetch（1s 超时），失败回退 shell curl。
 * @param path  API 路径（如 /connections）
 * @returns 响应文本
 * @throws 两种方式都失败时抛出
 */
const fetchClashApi = async (path: string): Promise<string> => {
  const url = `http://${apiHost.value}${path}`;
  try {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), 1000);
    const res = await fetch(url, {
      method: 'GET',
      headers: {
        'Authorization': `Bearer ${apiSecret.value}`,
        'Content-Type': 'application/json'
      },
      signal: controller.signal
    });
    clearTimeout(timer);
    if (res.ok) {
      return await res.text();
    }
  } catch (err) {
    console.warn(`Fetch Clash API failed for ${path}, falling back to curl:`, err);
  }

  // 原生 fetch 失败时回退到 shell curl
  const cmd = `curl -fsS --connect-timeout 1 -m 2 -H "Authorization: Bearer ${apiSecret.value}" "${url}"`;
  const ksuRes = await execAsync(cmd);
  if (ksuRes.errno === 0) {
    return ksuRes.stdout;
  }
  throw new Error(`Clash API request failed: ${ksuRes.stderr}`);
};

/** 高频轮询：仅拉 /connections 更新流量速度/总量。 */
const updateTrafficStats = async () => {
  try {
    const connRaw = await fetchClashApi('/connections');
    const connData = JSON.parse(connRaw);
    const dlTotal = connData.downloadTotal || connData.download || 0;
    const ulTotal = connData.uploadTotal || connData.upload || 0;
    updateTrafficSpeed(dlTotal, ulTotal);
  } catch (e) {
    console.warn('Failed to update traffic stats:', e);
  }
};

/** 低频轮询：拉 /proxies（当前节点）+ /configs（模式），变动不频繁。 */
const updateMetaStats = async () => {
  try {
    const [proxyRaw, configRaw] = await Promise.all([
      fetchClashApi('/proxies'),
      fetchClashApi('/configs')
    ]);

    try {
      const data = JSON.parse(proxyRaw);
      const proxyGroup = data.proxies?.Proxy || data.Proxy;
      if (proxyGroup && proxyGroup.now) {
        stats.value.currentNode = proxyGroup.now;
      }
    } catch (e) {
      console.warn('Failed to parse proxies data:', e);
    }

    try {
      const data = JSON.parse(configRaw);
      if (data.mode) {
        const runtimeMode = String(data.mode);
        stats.value.isWifiDirect = runtimeMode.toLowerCase() === 'wifidirect';
        if (!stats.value.isWifiDirect) {
          stats.value.mode = modeStringToNumber(runtimeMode);
        }
      }
    } catch (e) {
      console.warn('Failed to parse configs data:', e);
    }
  } catch (err) {
    console.warn('Failed to fetch Clash meta stats:', err);
  }
};

/**
 * 合并 pid 探测与系统/进程指标为单次 shell：输出首行 "PID <pid>" 或 "NORUN"，随后 4 段 proc 内容
 * (/proc/uptime、/proc/<pid>/statm、/proc/<pid>/stat、/proc/stat)，据此就地更新 CPU/内存/运行时长。
 * @returns pid（运行中）或 null（未运行）
 */
const loadRunningMetrics = async (): Promise<string | null> => {
  const cmd = [
    'pid=$(pidof sing-box 2>/dev/null | awk \'{print $1}\')',
    '[ -z "$pid" ] && { echo NORUN; exit 0; }',
    'echo "PID $pid"',
    'cat /proc/uptime "/proc/$pid/statm" "/proc/$pid/stat" /proc/stat'
  ].join('\n');

  let res;
  try {
    res = await execAsync(cmd);
  } catch (e) {
    console.warn('Failed to read running metrics:', e);
    return null;
  }
  if (res.errno !== 0 || !res.stdout) return null;

  const lines = res.stdout.split('\n');
  if (lines[0] === 'NORUN') return null;

  // 解析 pid 行
  const pid = lines[0].startsWith('PID ') ? lines[0].slice(4).trim() : '';
  if (!pid || lines.length < 5) return pid || null;

  try {
    // lines[1]=uptime, [2]=statm, [3]=stat, [4..]=/proc/stat
    const systemUptime = parseFloat(lines[1].split(/\s+/)[0]) || 0;

    const statmFields = lines[2].trim().split(/\s+/);
    const rss = parseInt(statmFields[1]) || 0;
    stats.value.memUsage = (rss * 4096 / systemTotalMem.value) * 100;

    const statLine = lines[3];
    const rparen = statLine.lastIndexOf(')');
    if (rparen !== -1) {
      const fieldsAfterName = statLine.substring(rparen + 1).trim().split(/\s+/);
      const utime = parseInt(fieldsAfterName[11]) || 0;
      const stime = parseInt(fieldsAfterName[12]) || 0;
      const procTicks = utime + stime;
      const startTimeTicks = parseInt(fieldsAfterName[19]) || 0;

      const totalCpuTicks = lines[4].trim().split(/\s+/).slice(1)
        .reduce((sum, val) => sum + (parseInt(val) || 0), 0);

      if (lastProcessTicks > 0 && lastTotalCpuTicks > 0) {
        const deltaProc = procTicks - lastProcessTicks;
        const deltaTotal = totalCpuTicks - lastTotalCpuTicks;
        if (deltaTotal > 0) {
          stats.value.cpuUsage = Math.min(100, Math.max(0, (deltaProc * 100 * cpuCores.value) / deltaTotal));
        }
      }
      lastProcessTicks = procTicks;
      lastTotalCpuTicks = totalCpuTicks;

      const processUptimeSec = systemUptime - (startTimeTicks / 100);
      const uptimeSec = Math.max(0, Math.floor(processUptimeSec));
      const hours = Math.floor(uptimeSec / 3600);
      const minutes = Math.floor((uptimeSec % 3600) / 60);
      const seconds = uptimeSec % 60;
      stats.value.uptime = [hours, minutes, seconds].map(n => String(n).padStart(2, '0')).join(':');
    }
  } catch (err) {
    console.warn('Failed to parse running metrics:', err);
  }

  return pid;
};

/** mock 环境下的伪随机统计更新（无真机时驱动界面动效）。 */
const runMockStatsUpdate = () => {
  if (!stats.value.isRunning) return;

  const dlNoise = (Math.random() - 0.45) * 800000;
  const ulNoise = (Math.random() - 0.45) * 60000;
  
  stats.value.downloadSpeed = Math.max(200000, stats.value.downloadSpeed + dlNoise);
  stats.value.uploadSpeed = Math.max(20000, stats.value.uploadSpeed + ulNoise);
  
  stats.value.downloadTotal += Math.floor(stats.value.downloadSpeed * 2);
  stats.value.uploadTotal += Math.floor(stats.value.uploadSpeed * 2);
  
  stats.value.cpuUsage = Math.max(1.2, Math.min(92, stats.value.cpuUsage + (Math.random() - 0.5) * 3));
  stats.value.memUsage = Math.max(30, Math.min(85, stats.value.memUsage + (Math.random() - 0.5) * 0.4));
  
  const parts = stats.value.uptime.split(':').map(Number);
  if (parts.length === 3) {
    let s = parts[2] + 2;
    let m = parts[1];
    let h = parts[0];
    if (s >= 60) { s -= 60; m++; }
    if (m >= 60) { m -= 60; h++; }
    stats.value.uptime = [h, m, s].map(n => String(n).padStart(2, '0')).join(':');
  }

  dlHistory.value.push(stats.value.downloadSpeed);
  ulHistory.value.push(stats.value.uploadSpeed);

  if (dlHistory.value.length > 40) dlHistory.value.shift();
  if (ulHistory.value.length > 40) ulHistory.value.shift();

  requestCanvasRedraw();
};

// 轮询周期计数：每 META_EVERY 个周期刷新一次低频元信息
let statsTick = 0;
const META_EVERY = 4;
let prevRunning = false;

// ===================================================================
// 统计轮询主循环
// ===================================================================

/** 轮询一次：读运行指标判断是否在运行；运行中高频拉流量、低频(或刚启动)拉节点/模式/内网 IP。 */
const updateStats = async () => {
  if (!isKsuEnv()) {
    runMockStatsUpdate();
    return;
  }

  try {
    // 单次 shell 同时拿到 pid + CPU/内存/运行时长
    const pid = await loadRunningMetrics();
    const isRunning = pid !== null;
    stats.value.isRunning = isRunning;

    if (!isRunning) {
      stats.value.isWifiDirect = false;
      stats.value.uptime = '服务未开启';
      stats.value.cpuUsage = 0;
      stats.value.memUsage = 0;
      stats.value.downloadSpeed = 0;
      stats.value.uploadSpeed = 0;
      servicePid.value = '';
      lastProcessTicks = 0;
      lastTotalCpuTicks = 0;
      lastDlTotal = 0;
      lastUlTotal = 0;
      lastTime = 0;
      dlHistory.value = Array(40).fill(0);
      ulHistory.value = Array(40).fill(0);
      resetPoints();
      requestCanvasRedraw();
      prevRunning = false;
      return;
    }

    servicePid.value = pid as string;

    // 高频：流量。低频(每 META_EVERY 周期 或 服务刚由停转启)：节点/模式/内网IP
    const justStarted = !prevRunning;
    const refreshMeta = justStarted || (statsTick % META_EVERY === 0);
    prevRunning = true;
    statsTick++;

    const tasks: Promise<void>[] = [updateTrafficStats()];
    if (refreshMeta) {
      tasks.push(updateMetaStats());
      tasks.push(loadInternalIp());
    }
    await Promise.all(tasks);

    dlHistory.value.push(stats.value.downloadSpeed);
    ulHistory.value.push(stats.value.uploadSpeed);
    if (dlHistory.value.length > 40) dlHistory.value.shift();
    if (ulHistory.value.length > 40) ulHistory.value.shift();
    requestCanvasRedraw();

  } catch (err) {
    console.error('Failed to update dashboard stats:', err);
  }
};

// ===================================================================
// 服务开关 / 模式切换 / 导航 动作
// ===================================================================

/** 切换服务启停：乐观更新开关，调用 CLI（启动用 detach 迁出冻结 cgroup），失败回滚。 */
const handleToggleService = async () => {
  if (isPending.value) return;
  const targetState = !stats.value.isRunning;
  stats.value.isRunning = targetState; // 乐观更新
  isPending.value = true;
  showToast(targetState ? t('dashboard.startingService') : t('dashboard.stoppingService'));
  
  try {
    if (isKsuEnv()) {
      if (targetState) {
        await runCli('service start', { detach: true });
      } else {
        await runCli('service stop');
      }
      await updateStats();
      showToast(targetState ? t('dashboard.serviceStarted') : t('dashboard.serviceStopped'));
    } else {
      setTimeout(() => {
        if (targetState) {
          stats.value.uptime = '00:00:00';
          stats.value.downloadSpeed = 2315680;
          stats.value.uploadSpeed = 142100;
        }
        showToast(targetState ? t('dashboard.serviceStarted') : t('dashboard.serviceStopped'));
        isPending.value = false;
      }, 1000);
      return;
    }
  } catch (err: any) {
    stats.value.isRunning = !targetState; // 出错时回滚乐观更新
    showToast(t('dashboard.operationFailed', { msg: err.message || err }));
  } finally {
    isPending.value = false;
  }
};

/** 打开出站模式选择弹窗。 */
const openModeDialog = () => {
  showModeDialog.value = true;
};

/** 关闭出站模式选择弹窗。 */
const closeModeDialog = () => {
  showModeDialog.value = false;
};

/**
 * 选择某个出站模式：先关弹窗再应用切换。
 * @param index  模式下标
 */
const selectMode = (index: number) => {
  closeModeDialog();
  handleModeChange(index);
};

/**
 * 切换出站模式：调用 CLI（detach 迁出冻结 cgroup）后刷新统计。
 * @param modeIndex  模式下标
 */
const handleModeChange = async (modeIndex: number) => {
  isPending.value = true;
  const modeStr = modeNumberToString(modeIndex);
  showToast(t('dashboard.switchingMode', { mode: modes.value[modeIndex] }));

  try {
    if (isKsuEnv()) {
      await runCli(`mode ${modeStr}`, { detach: true });
      await updateStats();
      showToast(t('dashboard.modeSwitched', { mode: modes.value[modeIndex] }));
    } else {
      setTimeout(() => {
        stats.value.isWifiDirect = false;
        stats.value.mode = modeIndex;
        showToast(t('dashboard.modeSwitched', { mode: modes.value[modeIndex] }));
        isPending.value = false;
      }, 500);
      return;
    }
  } catch (err: any) {
    showToast(t('dashboard.switchModeFailed', { msg: err.message || err }));
  } finally {
    isPending.value = false;
  }
};

/** 跳转到节点页（直连模式下无需选节点，仅提示）。 */
const navigateToNodes = () => {
  if (stats.value.mode !== 0) {
    emit('navigate', 'nodes');
  } else {
    showToast(t('dashboard.directNoNode'));
  }
};

// ===================================================================
// 流量曲线绘制（插值 + requestAnimationFrame 动画）
// ===================================================================

/** 把当前速度历史归一化为新的目标点集，并启动一段插值动画过渡到该点集。 */
const requestCanvasRedraw = () => {
  prevDownloadPoints = [...currentDownloadPoints];
  prevUploadPoints = [...currentUploadPoints];

  const maxPoints = 40;
  const maxSpeed = Math.max(
    ...dlHistory.value,
    ...ulHistory.value,
    100
  );

  currentDownloadPoints = dlHistory.value.map((speed, index) => ({
    x: index / (maxPoints - 1),
    y: speed / maxSpeed
  }));
  currentUploadPoints = ulHistory.value.map((speed, index) => ({
    x: index / (maxPoints - 1),
    y: speed / maxSpeed
  }));

  if (drawRequestFrame) {
    cancelAnimationFrame(drawRequestFrame);
  }
  chartAnimationStartTime = performance.now();
  drawRequestFrame = requestAnimationFrame(animateChart);
};

/** 动画帧回调：按 easeInOutQuad 缓动从上一帧点集插值到当前点集并逐帧重绘。 */
const animateChart = () => {
  const now = performance.now();
  const elapsed = now - chartAnimationStartTime;
  const t = Math.min(1, elapsed / chartAnimationDuration);

  // easeInOutQuad 缓动
  const easeT = t < 0.5 ? 2 * t * t : 1 - Math.pow(-2 * t + 2, 2) / 2;

  drawSpeedChartWithInterpolation(easeT);

  if (t < 1) {
    drawRequestFrame = requestAnimationFrame(animateChart);
  }
};

/**
 * 在上一帧与当前帧点集之间按进度 t 线性插值。
 * @param prev     上一帧点集
 * @param current  当前帧点集
 * @param t        进度 0~1
 * @returns 插值后的点集
 */
const interpolatePoints = (prev: Point[], current: Point[], t: number): Point[] => {
  if (current.length === 0) return [];
  if (prev.length === 0) return current;
  const result: Point[] = [];
  for (let i = 0; i < current.length; i++) {
    if (i >= prev.length) {
      result.push(current[i]);
    } else {
      const x = prev[i].x + (current[i].x - prev[i].x) * t;
      const y = prev[i].y + (current[i].y - prev[i].y) * t;
      result.push({ x, y });
    }
  }
  return result;
};

/**
 * 用二次贝塞尔把点集构建为平滑曲线 Path2D。
 * @param points       归一化点集（x,y ∈ 0~1）
 * @param w            画布宽
 * @param h            画布高
 * @param fillToBottom  true 时闭合到底部用于填充
 * @returns Path2D 路径
 */
const buildCurvePath = (points: Point[], w: number, h: number, fillToBottom: boolean): Path2D => {
  const path = new Path2D();
  if (points.length === 0) return path;

  path.moveTo(points[0].x * w, (1 - points[0].y) * h);

  for (let i = 1; i < points.length - 1; i++) {
    const currentPoint = points[i];
    const nextPoint = points[i + 1];
    const midX = (currentPoint.x + nextPoint.x) / 2;
    const midY = (currentPoint.y + nextPoint.y) / 2;
    path.quadraticCurveTo(
      currentPoint.x * w,
      (1 - currentPoint.y) * h,
      midX * w,
      (1 - midY) * h
    );
  }

  if (points.length > 1) {
    const last = points[points.length - 1];
    path.lineTo(last.x * w, (1 - last.y) * h);
    if (fillToBottom) {
      path.lineTo(last.x * w, h);
      path.lineTo(0, h);
      path.closePath();
    }
  }

  return path;
};

/**
 * 按插值进度 t 重绘下载/上传两条曲线（填充 + 描边，渐变按高度缓存）。
 * @param t  插值进度 0~1
 */
const drawSpeedChartWithInterpolation = (t: number) => {
  const canvas = speedCanvas.value;
  if (!canvas) return;

  const ctx = canvas.getContext('2d');
  if (!ctx) return;

  const width = canvas.clientWidth;
  const height = canvas.clientHeight;

  if (canvas.width !== width || canvas.height !== height) {
    canvas.width = width;
    canvas.height = height;
  }

  // 渐变仅在高度变化时重建
  if (height !== cachedGradH || !dlFillGrad || !ulFillGrad) {
    dlFillGrad = ctx.createLinearGradient(0, 0, 0, height);
    dlFillGrad.addColorStop(0, 'rgba(33, 150, 243, 0.25)');
    dlFillGrad.addColorStop(1, 'rgba(33, 150, 243, 0.00)');
    ulFillGrad = ctx.createLinearGradient(0, 0, 0, height);
    ulFillGrad.addColorStop(0, 'rgba(76, 175, 80, 0.15)');
    ulFillGrad.addColorStop(1, 'rgba(76, 175, 80, 0.00)');
    cachedGradH = height;
  }

  ctx.clearRect(0, 0, width, height);

  const interpolatedDownload = interpolatePoints(prevDownloadPoints, currentDownloadPoints, t);
  const interpolatedUpload = interpolatePoints(prevUploadPoints, currentUploadPoints, t);
  const strokeWidth = 2;

  // 1. 下载曲线：填充路径 + 描边路径各构建一次
  if (interpolatedDownload.length > 0) {
    ctx.fillStyle = dlFillGrad;
    ctx.fill(buildCurvePath(interpolatedDownload, width, height, true));
    ctx.strokeStyle = '#2196F3';
    ctx.lineWidth = strokeWidth;
    ctx.stroke(buildCurvePath(interpolatedDownload, width, height, false));
  }

  // 2. 上传曲线
  if (interpolatedUpload.length > 0) {
    ctx.fillStyle = ulFillGrad;
    ctx.fill(buildCurvePath(interpolatedUpload, width, height, true));
    ctx.strokeStyle = '#4CAF50';
    ctx.lineWidth = strokeWidth;
    ctx.stroke(buildCurvePath(interpolatedUpload, width, height, false));
  }
};

// ===================================================================
// 初始化与生命周期
// ===================================================================

/** 首次初始化：加载硬件常量、API 配置、内网 IP。 */
const initDashboard = async () => {
  await loadHardwareConstants();
  await loadApiConfig();
  await loadInternalIp();
};

/** 启动统计轮询（1.5s 间隔，幂等：避免重复定时器）。 */
const startStatsPolling = () => {
  if (statsTimer) return;
  statsTimer = setInterval(updateStats, 1500) as any;
};

/** 停止统计轮询。 */
const stopStatsPolling = () => {
  if (statsTimer) {
    clearInterval(statsTimer);
    statsTimer = null as any;
  }
};

onMounted(async () => {
  await initDashboard();
  await updateStats();
  startStatsPolling();
  window.addEventListener('resize', requestCanvasRedraw);
  setTimeout(requestCanvasRedraw, 200);
});

// keep-alive 缓存下：切回本页时恢复轮询并立即刷新一次
onActivated(() => {
  updateStats();
  startStatsPolling();
  requestCanvasRedraw();
});

// 切走时暂停轮询，避免后台持续执行 shell 命令
onDeactivated(() => {
  stopStatsPolling();
});

onUnmounted(() => {
  stopStatsPolling();
  if (drawRequestFrame) cancelAnimationFrame(drawRequestFrame);
  window.removeEventListener('resize', requestCanvasRedraw);
});

watch(() => stats.value.isRunning, (newVal) => {
  if (!newVal) {
    dlHistory.value = Array(40).fill(0);
    ulHistory.value = Array(40).fill(0);
    resetPoints();
    requestCanvasRedraw();
  }
});
</script>

<template>
  <div class="dashboard-grid animated-fade-in">
    <!-- 服务状态卡片（含实时速度曲线） -->
    <div class="miuix-card">
      <div class="pref-row">
        <div class="pref-left">
          <div class="pref-text-container">
            <span class="pref-title">{{ t('dashboard.serviceStatus') }}</span>
            <span class="pref-desc" :class="{ 'running-desc': stats.isRunning }">
              {{ stats.isRunning ? t('dashboard.uptimePrefix') + ': ' + stats.uptime : t('dashboard.serviceOff') }}
            </span>
          </div>
        </div>
        <div class="pref-right">
          <md-switch 
            :selected="stats.isRunning" 
            :disabled="isPending"
            @click.stop.prevent="handleToggleService">
          </md-switch>
        </div>
      </div>

      <!-- 实时速度曲线可折叠区（运行时展开） -->
      <div class="speed-chart-container" v-show="stats.isRunning">
        <div class="speed-row">
          <div class="speed-badge dl">
            <md-icon class="speed-badge-icon">
              <svg viewBox="0 0 24 24">
                <path d="M20 12l-1.41-1.41L13 16.17V4h-2v12.17L5.41 10.59L4 12l8 8 8-8z" />
              </svg>
            </md-icon>
            <span>{{ downloadSpeedText }}</span>
          </div>
          <div class="speed-badge ul">
            <md-icon class="speed-badge-icon">
              <svg viewBox="0 0 24 24">
                <path d="M4 12l1.41 1.41L11 7.83V20h2V7.83l5.59 5.59L20 12l-8-8-8 8z" />
              </svg>
            </md-icon>
            <span>{{ uploadSpeedText }}</span>
          </div>
        </div>
        <canvas ref="speedCanvas" class="speed-canvas"></canvas>
      </div>
    </div>

    <!-- 网络信息卡片 -->
    <div class="miuix-card" v-if="stats.isRunning">
      <!-- 内网 IP 行 -->
      <div class="pref-row">
        <div class="pref-left">
          <div class="pref-icon-container">
            <md-icon>
              <svg viewBox="0 0 24 24">
                <path d="M19 13H5c-1.1 0-2 .9-2 2v4c0 1.1.9 2 2 2h14c1.1 0 2-.9 2-2v-4c0-1.1-.9-2-2-2zM19 3H5c-1.1 0-2 .9-2 2v4c0 1.1.9 2 2 2h14c1.1 0 2-.9 2-2V5c0-1.1-.9-2-2-2zM9 18H7v-2h2v2zm0-10H7V6h2v2z" />
              </svg>
            </md-icon>
          </div>
          <div class="pref-text-container">
            <span class="pref-title">{{ t('dashboard.lanIp') }}</span>
          </div>
        </div>
        <div class="pref-right">
          <span class="pref-value val-mono">{{ stats.internalIp }}</span>
        </div>
      </div>

      <div class="pref-divider"></div>

      <!-- 总流量行 -->
      <div class="pref-row">
        <div class="pref-left">
          <div class="pref-icon-container">
            <md-icon>
              <svg viewBox="0 0 24 24">
                <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-1 17.93c-3.95-.49-7-3.85-7-7.93 0-.62.08-1.21.21-1.79L9 15v1c0 1.1.9 2 2 2v1.93zm6.9-2.54c-.26-.81-1-1.39-1.9-1.39h-1v-3c0-.55-.45-1-1-1H8v-2h2c.55 0 1-.45 1-1V7h2c1.1 0 2-.9 2-2v-.41c2.93 1.19 5 4.06 5 7.41 0 2.08-.8 3.97-2.1 5.39z"/>
              </svg>
            </md-icon>
          </div>
          <div class="pref-text-container">
            <span class="pref-title">{{ t('dashboard.totalTraffic') }}</span>
          </div>
        </div>
        <div class="pref-right">
          <span class="pref-value val-mono">↑ {{ uploadTotalText }}   ↓ {{ downloadTotalText }}</span>
        </div>
      </div>
    </div>

    <!-- 系统指标卡片行 -->
    <div class="metrics-row" v-if="stats.isRunning">
      <!-- CPU 占用卡片 -->
      <div class="metric-card">
        <div class="metric-header">
          <md-icon class="metric-icon">
            <svg viewBox="0 0 24 24">
              <path d="M15 9H9v6h6V9zm-2 4h-2v-2h2v2zm8-2V9h-2V7c0-1.1-.9-2-2-2h-2V3h-2v2H9V3H7v2H5c-1.1 0-2 .9-2 2v2H1v2h2v2H1v2h2v2c0 1.1.9 2 2 2h2v2h2v-2h2v2h2v-2h2v-2h2v-2h2v-2h-2v-2h2zm-4 6H5V7h14v10z" />
            </svg>
          </md-icon>
          <span class="metric-label">{{ t('dashboard.cpuUsage') }}</span>
        </div>
        <span class="metric-value">{{ cpuText }}%</span>
      </div>

      <!-- 内存占用卡片 -->
      <div class="metric-card">
        <div class="metric-header">
          <md-icon class="metric-icon">
            <svg viewBox="0 0 24 24">
              <path d="M2 20h20v-4H2v4zm2-3h2v2H4v-2zM2 4v4h20V4H2zm4 3H4V5h2v2zm-4 7h20v-4H2v4zm2-3h2v2H4v-2z" />
            </svg>
          </md-icon>
          <span class="metric-label">{{ t('dashboard.memUsage') }}</span>
        </div>
        <span class="metric-value">{{ memText }}%</span>
      </div>
    </div>

    <!-- 出站设置卡片 -->
    <div class="miuix-card" v-if="stats.isRunning">
      <!-- 出站模式选择行 -->
      <div class="pref-row clickable" @click="openModeDialog">
        <md-ripple></md-ripple>
        <div class="pref-left">
          <div class="pref-icon-container">
            <md-icon>
              <svg viewBox="0 0 24 24">
                <path d="M14 4l2.29 2.29-2.88 2.88 1.42 1.42 2.88-2.88L20 12V4h-8zM10 4H4v8l2.29-2.29 4.71 4.7V20h2v-6.41l-5.29-5.3L10 6z" />
              </svg>
            </md-icon>
          </div>
          <div class="pref-text-container">
            <span class="pref-title">{{ t('dashboard.outboundMode') }}</span>
            <span
              class="pref-desc"
              :class="{ 'wifi-direct-hint': stats.isWifiDirect }"
              :role="stats.isWifiDirect ? 'status' : undefined">
              <span v-if="stats.isWifiDirect" class="status-dot" aria-hidden="true"></span>
              {{ currentModeDescription }}
            </span>
          </div>
        </div>
        <div class="pref-right">
          <span class="pref-value" :class="{ 'wifi-direct-value': stats.isWifiDirect }">{{ currentModeLabel }}</span>
          <div class="pref-arrow-icon">
            <md-icon>
              <svg viewBox="0 0 24 24">
                <path d="M8.59 16.59L13.17 12 8.59 7.41 10 6l6 6-6 6-1.41-1.41z" />
              </svg>
            </md-icon>
          </div>
        </div>
      </div>

      <div class="pref-divider"></div>

      <!-- 当前节点行 -->
      <div class="pref-row clickable" @click="navigateToNodes">
        <md-ripple></md-ripple>
        <div class="pref-left">
          <div class="pref-icon-container">
            <md-icon>
              <svg viewBox="0 0 24 24">
                <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-1 17.93c-3.95-.49-7-3.85-7-7.93 0-.62.08-1.21.21-1.79L9 15v1c0 1.1.9 2 2 2v1.93zm6.9-2.54c-.26-.81-1-1.39-1.9-1.39h-1v-3c0-.55-.45-1-1-1H8v-2h2c.55 0 1-.45 1-1V7h2c1.1 0 2-.9 2-2v-.41c2.93 1.19 5 4.06 5 7.41 0 2.08-.8 3.97-2.1 5.39z" />
              </svg>
            </md-icon>
          </div>
          <div class="pref-text-container">
            <span class="pref-title">{{ t('dashboard.currentNode') }}</span>
            <span class="pref-desc">{{ stats.mode === 0 ? t('dashboard.modeDirect') : (stats.currentNode || t('dashboard.nodeUnselected')) }}</span>
          </div>
        </div>
        <div class="pref-right">
          <div class="pref-arrow-icon">
            <md-icon>
              <svg viewBox="0 0 24 24">
                <path d="M8.59 16.59L13.17 12 8.59 7.41 10 6l6 6-6 6-1.41-1.41z" />
              </svg>
            </md-icon>
          </div>
        </div>
      </div>
    </div>

    <!-- 出站模式选择弹窗 -->
    <div class="dialog-container">
      <md-dialog :open="showModeDialog" @close="showModeDialog = false" class="transparent-scrim">
        <div slot="headline">{{ t('dashboard.outboundMode') }}</div>
        <div slot="content" class="dialog-list-container">
          <div v-if="stats.isWifiDirect" class="wifi-direct-dialog-notice">
            <span class="status-dot" aria-hidden="true"></span>
            <div>
              <div class="wifi-direct-dialog-title">{{ t('dashboard.wifiDirectActive') }}</div>
              <div class="wifi-direct-dialog-desc">{{ t('dashboard.wifiDirectAutoHint') }}</div>
            </div>
          </div>
          <md-list>
            <md-list-item
              v-for="(name, index) in modes"
              :key="index"
              type="button"
              class="dialog-list-item"
              @click="selectMode(index)">
              <div slot="headline">{{ name }}</div>
              <div slot="supporting-text">{{ modeDescriptions[index] }}</div>
              <md-icon slot="end" v-if="!stats.isWifiDirect && stats.mode === index" class="dialog-check-icon">
                <svg viewBox="0 0 24 24">
                  <path d="M9 16.17L4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41z" />
                </svg>
              </md-icon>
            </md-list-item>
          </md-list>
        </div>
        <div slot="actions">
          <md-text-button @click="closeModeDialog">{{ t('common.cancel') }}</md-text-button>
        </div>
      </md-dialog>
    </div>
  </div>
</template>

<style scoped>
.dashboard-grid {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding-bottom: 24px;
}

/* Miuix 风格的无边框布局卡片 */
.miuix-card {
  background-color: var(--md-sys-color-surface-container);
  border-radius: var(--radius-xl);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

/* 偏好行样式（仿 Compose 布局） */
.pref-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 16px;
  background: transparent;
  border: none;
  text-align: left;
  width: 100%;
  color: inherit;
  position: relative;
  overflow: hidden;
}

.pref-row.clickable {
  cursor: pointer;
}

.pref-left {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-grow: 1;
}

.pref-icon-container {
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--md-sys-color-primary);
}

.pref-icon-container md-icon {
  --md-icon-size: 20px;
  display: block;
}

.pref-icon-container md-icon svg {
  width: 20px;
  height: 20px;
  fill: currentColor;
}

.pref-text-container {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.pref-title {
  font-size: 15px;
  font-weight: 500;
  color: var(--md-sys-color-on-surface);
  line-height: 1.2;
}

.pref-desc {
  font-size: 12px;
  color: var(--md-sys-color-on-surface-variant);
  line-height: 1.35;
}

.wifi-direct-hint {
  display: flex;
  align-items: center;
  gap: 6px;
  color: var(--md-sys-color-primary);
  font-weight: 500;
}

.status-dot {
  width: 6px;
  height: 6px;
  flex: 0 0 6px;
  border-radius: 50%;
  background-color: currentColor;
  box-shadow: 0 0 0 3px color-mix(in srgb, currentColor 14%, transparent);
}

.running-desc {
  color: var(--md-sys-color-primary);
  font-weight: 500;
}

.pref-right {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--md-sys-color-on-surface-variant);
}

.pref-value {
  font-size: 14px;
  color: var(--md-sys-color-on-surface-variant);
}

.wifi-direct-value {
  padding: 4px 9px;
  border-radius: 999px;
  color: var(--md-sys-color-primary);
  background-color: var(--md-sys-color-primary-container);
  font-size: 12px;
  font-weight: 600;
  white-space: nowrap;
}

.pref-arrow-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--md-sys-color-on-surface-variant);
  opacity: 0.6;
}

.pref-arrow-icon md-icon {
  --md-icon-size: 20px;
  display: block;
}

.pref-arrow-icon md-icon svg {
  width: 20px;
  height: 20px;
  fill: currentColor;
}

.pref-divider {
  height: 1px;
  background-color: var(--md-sys-color-outline-variant);
  margin: 0 16px;
  opacity: 0.5;
}

/* 可折叠的速度曲线区 */
.speed-chart-container {
  padding: 0 16px 16px 16px;
}

.speed-row {
  display: flex;
  justify-content: space-between;
  margin-bottom: 12px;
  padding: 0 4px;
}

.speed-badge {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  color: var(--md-sys-color-on-surface-variant);
}

.speed-badge-icon {
  --md-icon-size: 14px;
  display: block;
}

.speed-badge-icon svg {
  width: 14px;
  height: 14px;
  fill: currentColor;
}

.speed-badge.dl {
  color: #2196F3;
}

.speed-badge.ul {
  color: #4CAF50;
}

.speed-canvas {
  width: 100%;
  height: 112px;
  display: block;
}

.val-mono {
  font-family: var(--md-ref-typeface-mono);
  font-size: 13px;
}

/* 系统指标行（两列） */
.metrics-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}

.metric-card {
  background-color: var(--md-sys-color-surface-container);
  border-radius: var(--radius-xl);
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 8px;
  position: relative;
  overflow: hidden;
}

.metric-icon-bg {
  position: absolute;
  bottom: -10px;
  right: -10px;
  opacity: 0.04;
  color: var(--md-sys-color-on-surface);
}

.metric-icon-bg md-icon {
  --md-icon-size: 80px;
  display: block;
}

.metric-icon-bg md-icon svg {
  width: 80px;
  height: 80px;
  fill: currentColor;
}

.metric-header {
  display: flex;
  align-items: center;
  gap: 4px;
  z-index: 1;
}

.metric-icon {
  --md-icon-size: 16px;
  display: block;
  color: var(--md-sys-color-primary);
}

.metric-icon svg {
  width: 16px;
  height: 16px;
  fill: currentColor;
}

.metric-label {
  font-size: 13px;
  font-weight: 500;
  color: var(--md-sys-color-on-surface-variant);
}

.metric-value {
  font-size: 18px;
  font-weight: bold;
  color: var(--md-sys-color-on-surface);
  z-index: 1;
}

/* 选择弹窗样式 */
.dialog-container {
  position: relative;
  z-index: 500;
}

.transparent-scrim {
  --md-dialog-scrim-color: transparent;
  --md-sys-color-scrim: transparent;
}

.dialog-list-container {
  padding: 0 !important;
  min-width: 280px;
}

.wifi-direct-dialog-notice {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  margin-bottom: 10px;
  padding: 12px 14px;
  border-radius: var(--radius-md);
  color: var(--md-sys-color-on-primary-container);
  background-color: var(--md-sys-color-primary-container);
}

.wifi-direct-dialog-notice .status-dot {
  margin-top: 6px;
}

.wifi-direct-dialog-title {
  font-size: 14px;
  font-weight: 600;
  line-height: 1.3;
}

.wifi-direct-dialog-desc {
  margin-top: 3px;
  font-size: 12px;
  line-height: 1.4;
  opacity: 0.82;
}

md-list {
  padding: 0;
  --md-list-container-color: transparent;
  border-radius: var(--md-sys-shape-corner-large);
  overflow: clip;
}

md-list-item {
  background-color: var(--md-sys-color-surface-container);
  border-radius: var(--radius-xs);
  cursor: pointer;
  margin-bottom: 2px;
}

md-list-item:last-child {
  margin-bottom: 0;
}

.dialog-check-icon {
  color: var(--md-sys-color-primary);
}
</style>
