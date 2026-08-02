/**
 * @file ksu.ts
 * @description WebUI 与 KernelSU/Magisk/APatch root 环境的桥接层（平台抽象 / 数据访问层）。
 *   所有页面统一通过本模块执行 shell、读写文件、读取应用列表、调用原生 UI 能力，
 *   不直接接触底层 API。每个能力都内置「真机 root / 浏览器 mock」双轨：
 *   非 KernelSU 环境下返回模拟数据，便于 `npm run dev` 开发整套界面。
 *
 *   底层依赖官方 `kernelsu` 库（v3.0.2）暴露的 API：
 *   exec / spawn / toast / fullScreen / enableEdgeToEdge / moduleInfo /
 *   listPackages / getPackagesInfo / exit。
 */
import {
  exec,
  spawn,
  toast,
  fullScreen,
  enableEdgeToEdge,
  moduleInfo,
  listPackages,
  getPackagesInfo,
  exit
} from 'kernelsu';

// ===================================================================
// 一、环境探测
// ===================================================================

/**
 * 是否运行在 KernelSU WebView（真机 root）环境内。
 * 用于在所有 API 中切换「真机调用 / 浏览器 mock」。
 * @returns true=真机 root 环境；false=普通浏览器（开发态）
 */
export const isKsuEnv = (): boolean => {
  return typeof (window as any).ksu !== 'undefined' || typeof (window as any).KSU !== 'undefined';
};

// ===================================================================
// 二、底层执行：shell 命令
// ===================================================================

/** shell 执行结果（与 kernelsu exec 返回结构一致）。 */
export interface ExecResults {
  errno: number;   // 退出码，0 为成功
  stdout: string;  // 标准输出
  stderr: string;  // 标准错误
}

/** 执行选项（透传给 kernelsu exec）。 */
export interface ExecOptions {
  cwd?: string;                        // 工作目录
  env?: Record<string, string>;        // 环境变量
}

/**
 * 在 root shell 中异步执行命令，永不抛出（失败以非零 errno 返回）。
 * 非真机环境返回 mock 结果。
 * @param cmd  完整命令（含空格分隔的参数）
 * @param options  可选 cwd / env
 * @returns Promise<ExecResults>
 */
export const execAsync = async (cmd: string, options?: ExecOptions): Promise<ExecResults> => {
  if (!isKsuEnv()) {
    console.warn(`[KSU Mock execAsync] ${cmd}`);
    return { errno: 0, stdout: 'Mock stdout', stderr: '' };
  }
  try {
    return await exec(cmd, options ?? {});
  } catch (err: any) {
    return { errno: -1, stdout: '', stderr: err?.message || String(err) };
  }
};

/**
 * 在 root shell 中以流式方式运行命令（基于 kernelsu spawn）。
 * 适合长输出 / 实时日志：逐块回调 stdout/stderr，结束时回调退出码。
 * 非真机环境为空操作（仅打印 mock 日志并立即回调 exit 0）。
 * @param command  命令名
 * @param args     参数数组
 * @param handlers onStdout / onStderr / onExit / onError 回调
 * @param options  可选 cwd / env
 */
export const spawnStream = (
  command: string,
  args: string[] = [],
  handlers: {
    onStdout?: (chunk: string) => void;
    onStderr?: (chunk: string) => void;
    onExit?: (code: number) => void;
    onError?: (err: any) => void;
  } = {},
  options?: ExecOptions
): void => {
  if (!isKsuEnv()) {
    console.warn(`[KSU Mock spawnStream] ${command} ${args.join(' ')}`);
    handlers.onExit?.(0);
    return;
  }
  try {
    const child = spawn(command, args, options ?? {});
    if (handlers.onStdout) child.stdout.on('data', handlers.onStdout);
    if (handlers.onStderr) child.stderr.on('data', handlers.onStderr);
    if (handlers.onExit) child.on('exit', handlers.onExit);
    if (handlers.onError) child.on('error', handlers.onError);
  } catch (err) {
    handlers.onError?.(err);
  }
};

/** 应用包信息（与 kernelsu getPackagesInfo 返回结构一致）。 */
export interface PackagesInfo {
  packageName: string;   // 包名
  versionName: string;   // 版本名
  versionCode: number;   // 版本号
  appLabel: string;      // 显示名称
  isSystem: boolean;     // 是否系统应用
  uid: number;           // 应用 UID
}

// ===================================================================
// 三、模块 CLI 调用
// ===================================================================

/**
 * 调用模块的 scripts/cli 命令并返回 stdout。
 * 优先用绝对路径；仅当脚本无法定位/执行时才回退相对路径，
 * 脚本已运行但返回非零（合法的命令错误）直接抛出，避免有副作用的命令被重复执行。
 * 非真机环境返回各命令的 mock 输出。
 * @param subCommand  cli 子命令，如 'service start' / 'node list'
 * @param opts.detach 用 su 包裹执行：复用 KernelSU su 的 switch_cgroups，
 *                    使其启动的常驻进程(sing-box)迁出管理器冻结 cgroup，
 *                    切后台不被冻结而断网。仅对会拉起常驻进程的命令(如
 *                    service start/restart、mode)需要。
 * @returns Promise<string> 命令标准输出
 * @throws 命令执行失败时抛出 Error
 */
export const runCli = async (subCommand: string, opts: { detach?: boolean } = {}): Promise<string> => {
  if (!isKsuEnv()) {
    console.warn(`[KSU Mock CLI] Executing: sh ./scripts/cli ${subCommand}`);
    // 模拟各命令的返回，便于浏览器调试
    if (subCommand.includes('service status')) {
      return `服务状态:\n运行状态: \x1b[0;32m运行中\x1b[0m\n进程 PID: 12345\n运行时间: 3600 秒\n当前节点: Proxy-HK-01.json\n出站模式: rule\n透明代理: eBPF (DNS=hijack IPv6=1 共享网络=0)\n运行模式: Rule\n内核版本: sing-box version 1.14.0`;
    }
    if (subCommand.includes('ebpf status')) {
      return `eBPF 入站配置:\n代理协议: TCP+UDP\nDNS 模式: hijack\nIPv6: 1\n共享网络: 0\n分应用代理: 1`;
    }
    if (subCommand.includes('app list')) {
      return `分应用代理: 1\n应用模式: blacklist\n代理列表: com.google.android.youtube com.twitter.android\n绕过列表: com.tencent.mm com.eg.android.Alipay`;
    }
    if (subCommand.includes('sub list')) {
      return `订阅列表:\n  - Sub1 (45 个节点，更新于 2026-06-04T12:00:00Z)\n  - Sub2 (120 个节点，更新于 2026-06-03T18:30:00Z)`;
    }
    if (subCommand.includes('service logs')) {
      return `[2026-06-04 16:00:00] [INFO] sing-box service started successfully\n[2026-06-04 16:00:01] [INFO] inbound/tun: listening on tun0\n[2026-06-04 16:00:02] [INFO] dns: router initialized\n[2026-06-04 16:05:00] [INFO] connection: 127.0.0.1:45326 -> 1.1.1.1:53 (Proxy-HK-01)`;
    }
    return 'Mock output';
  }

  // detach=true 时用 su 包裹：触发 KernelSU su 的 switch_cgroups，
  // 让被拉起的 sing-box 迁出管理器冻结 cgroup（命令为静态路径，无引号风险）。
  const wrap = (sh: string) => (opts.detach ? `su -c '${sh}'` : sh);

  // 先尝试绝对路径（system root 下最可靠）
  const fullPath = '/data/adb/modules/netproxy/scripts/cli';
  const res = await execAsync(wrap(`sh ${fullPath} ${subCommand}`));
  if (res.errno === 0) {
    return res.stdout;
  }

  // 仅当脚本本身无法定位/执行时才回退相对路径；
  // 脚本已运行但返回非零(合法的命令错误)直接上抛，避免有副作用的命令被重复执行
  const looksMissing =
    res.errno === 127 ||
    /can'?t open|No such file|not found|cannot execute/i.test(res.stderr || '');
  if (!looksMissing) {
    throw new Error(res.stderr || `CLI returned exit code ${res.errno}`);
  }

  // 回退到相对路径执行脚本
  const rel = await execAsync(wrap(`sh ./scripts/cli ${subCommand}`));
  if (rel.errno !== 0) {
    throw new Error(rel.stderr || `CLI returned exit code ${rel.errno}`);
  }
  return rel.stdout;
};

// ===================================================================
// 四、文件读写
// ===================================================================

/**
 * 写文件：内容先 base64 编码再 `base64 -d` 落地，规避 shell 转义问题
 * （中文 / 引号 / 换行 / 特殊字符均安全）。
 * 非真机环境写入 localStorage 模拟持久化。
 * @param path     目标文件绝对路径
 * @param content  文件内容（UTF-8）
 * @throws 写入失败时抛出 Error
 */
export const writeFileContent = async (path: string, content: string): Promise<void> => {
  if (!isKsuEnv()) {
    console.warn(`[KSU Mock Write] Path: ${path}\nContent:`, content);
    localStorage.setItem(`mock_file_${path}`, content);
    return;
  }

  try {
    // Unicode-safe base64 conversion in browser environment
    const utf8Bytes = new TextEncoder().encode(content);
    let binary = '';
    const len = utf8Bytes.byteLength;
    for (let i = 0; i < len; i++) {
      binary += String.fromCharCode(utf8Bytes[i]);
    }
    const base64Content = btoa(binary);
    
    const cmd = `echo "${base64Content}" | base64 -d > "${path}"`;
    const res = await execAsync(cmd);
    if (res.errno !== 0) {
      throw new Error(res.stderr || `Failed to write file to ${path}`);
    }
  } catch (err: any) {
    throw new Error(`Write error: ${err.message || err}`);
  }
};

/**
 * 读文件内容（cat）。非真机环境优先读 localStorage，否则返回内置 mock 模板。
 * @param path  文件绝对路径
 * @returns Promise<string> 文件内容
 * @throws 读取失败时抛出 Error
 */
export const readFileContent = async (path: string): Promise<string> => {
  if (!isKsuEnv()) {
    console.warn(`[KSU Mock Read] Path: ${path}`);
    const local = localStorage.getItem(`mock_file_${path}`);
    if (local !== null) return local;

    // 返回内置 mock 配置模板
    if (path.includes('06_route.json')) {
      return JSON.stringify({
        "rules": [
          { "ip_cidr": ["223.5.5.5/32"], "outbound": "direct" },
          { "domain_suffix": [".cn", ".zh"], "outbound": "direct" }
        ]
      }, null, 2);
    }
    if (path.includes('03_dns.json')) {
      return JSON.stringify({
        "servers": [
          { "tag": "dns_direct", "address": "223.5.5.5", "detour": "direct" },
          { "tag": "dns_proxy", "address": "8.8.8.8", "detour": "Proxy" }
        ]
      }, null, 2);
    }
    return '{}';
  }

  const res = await execAsync(`cat "${path}"`);
  if (res.errno !== 0) {
    throw new Error(res.stderr || `Failed to read file from ${path}`);
  }
  return res.stdout;
};

// ===================================================================
// 五、应用列表
// ===================================================================

/**
 * 获取应用图标 URL（KernelSU 提供的 ksu://icon/ 协议）。
 * 可直接用于 <img :src="getAppIconUrl(pkg)">。
 * @param packageName  包名
 * @returns 图标 URL
 */
export const getAppIconUrl = (packageName: string): string => {
  return `ksu://icon/${packageName}`;
};

/**
 * 获取已安装应用列表（含名称 / 版本 / UID / 是否系统应用）。
 * 真机直接走 KernelSU 原生 listPackages + getPackagesInfo（无 pm fork、有类型）。
 * 注：原生 API 仅返回主用户应用；不含工作资料 / 应用分身等多用户条目。
 * 非真机环境返回 mock 列表。
 * @param filter  'user' | 'system' | 'all'（默认 'all'）
 * @returns Promise<PackagesInfo[]>
 */
export const getAppPackagesList = async (filter: 'user' | 'system' | 'all' = 'all'): Promise<PackagesInfo[]> => {
  if (!isKsuEnv()) {
    console.warn(`[KSU Mock listPackages] filter=${filter}`);
    return [
      { packageName: 'com.google.android.youtube', versionName: '19.01.01', versionCode: 12345, appLabel: 'YouTube', isSystem: true, uid: 10123 },
      { packageName: 'com.twitter.android', versionName: '10.2.0', versionCode: 67890, appLabel: 'Twitter/X', isSystem: false, uid: 10456 },
      { packageName: 'com.tencent.mm', versionName: '8.0.48', versionCode: 11223, appLabel: '微信', isSystem: false, uid: 10888 },
      { packageName: 'com.eg.android.Alipay', versionName: '10.5.80', versionCode: 44556, appLabel: '支付宝', isSystem: false, uid: 10999 },
      { packageName: 'com.android.settings', versionName: '14.0', versionCode: 99999, appLabel: '设置', isSystem: true, uid: 10000 }
    ].filter(app => filter === 'all' || (filter === 'system' && app.isSystem) || (filter === 'user' && !app.isSystem));
  }

  try {
    // 1. 原生列出包名（type: user/system/all）
    const names = listPackages(filter);
    if (!names || names.length === 0) return [];

    // 2. 原生批量取详情（含 uid，已是真实 UID）
    const infos = getPackagesInfo(names) as PackagesInfo[];
    if (!infos) return [];

    // 3. 过滤掉无效条目（getPackagesInfo 对找不到的包返回 {packageName, error}）
    return infos.filter(info => info && info.packageName && typeof info.uid === 'number');
  } catch (err) {
    console.error('Failed to query apps via KernelSU listPackages/getPackagesInfo:', err);
    return [];
  }
};

// ===================================================================
// 六、ebpf.conf 配置读写
// ===================================================================

/** 按需为配置值补引号（含空格或为空时强制加引号）。 */
const formatValue = (value: string, forceQuotes: boolean): string => {
  const needsQuotes = forceQuotes || value.includes(' ') || value === '';
  if (!needsQuotes) return value;
  const escaped = value
    .replace(/\\/g, '\\\\')
    .replace(/"/g, '\\"')
    .replace(/\$/g, '\\$')
    .replace(/`/g, '\\`');
  return `"${escaped}"`;
};

const updateContent = (content: string, key: string, formattedValue: string): string => {
  const lines = content === '' ? [] : content.replace(/\r\n/g, '\n').split('\n');
  const searchPrefix = `${key}=`;

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    if (!line.startsWith(searchPrefix)) continue;

    const suffix = line.substring(searchPrefix.length);
    const commentIndex = suffix.indexOf('#');
    if (commentIndex >= 0) {
      lines[i] = `${searchPrefix}${formattedValue} ${suffix.substring(commentIndex)}`.trimEnd();
    } else {
      lines[i] = `${searchPrefix}${formattedValue}`;
    }
    return lines.join('\n');
  }

  lines.push(`${searchPrefix}${formattedValue}`);
  return lines.join('\n');
};

/**
 * 写入 ebpf.conf 的单个键值（保留行尾注释；键不存在则追加）。
 * 仅写文件，不触发核心重启，便于连续修改后一次性应用。
 * @param key  配置键，如 'APP_PROXY_ENABLE'
 * @param value  值
 * @param forceQuotes  是否强制加引号（列表 / 含空格值用）
 */
export const writeEbpfValue = async (key: string, value: string, forceQuotes: boolean = false): Promise<void> => {
  if (!isKsuEnv()) {
    console.log(`[KSU Mock WriteEbpfValue] Key: ${key}, Value: ${value}, forceQuotes: ${forceQuotes}`);
    if (key === 'APP_PROXY_ENABLE') {
      localStorage.setItem('mock_proxy_enabled', value === '1' ? 'true' : 'false');
    } else if (key === 'APP_PROXY_MODE') {
      localStorage.setItem('mock_proxy_mode', value);
    }
    return;
  }
  const filePath = '/data/adb/modules/netproxy/config/ebpf/ebpf.conf';
  const content = await readFileContent(filePath);
  const formattedValue = formatValue(value, forceQuotes);
  const updatedContent = updateContent(content, key, formattedValue);
  await writeFileContent(filePath, updatedContent);
};

export interface EbpfConfigState {
  appProxyEnabled: boolean;
  appProxyMode: 'blacklist' | 'whitelist';
  proxiedAppItems: string[];
}

/**
 * 解析 ebpf.conf，返回分应用代理的状态。
 * @returns 启用状态 / 模式 / 当前模式对应的应用列表（whitelist→代理列表，否则绕过列表）
 */
export const getEbpfConfigState = async (): Promise<EbpfConfigState> => {
  if (!isKsuEnv()) {
    const enabled = localStorage.getItem('mock_proxy_enabled') !== 'false';
    const mode = (localStorage.getItem('mock_proxy_mode') || 'blacklist') as 'blacklist' | 'whitelist';
    const storedChecked = localStorage.getItem('mock_checked_apps');
    const proxiedAppItems = storedChecked ? JSON.parse(storedChecked) : [];
    return {
      appProxyEnabled: enabled,
      appProxyMode: mode,
      proxiedAppItems
    };
  }

  const filePath = '/data/adb/modules/netproxy/config/ebpf/ebpf.conf';
  const content = await readFileContent(filePath);

  const lines = content.split('\n');
  let enabled = false;
  let mode: 'blacklist' | 'whitelist' = 'blacklist';
  let proxyList: string[] = [];
  let bypassList: string[] = [];

  const stripQuotesAndComments = (val: string): string => {
    let cleaned = val.trim();
    if (cleaned.startsWith('"') && cleaned.endsWith('"')) {
      cleaned = cleaned.substring(1, cleaned.length - 1);
    } else if (cleaned.startsWith("'") && cleaned.endsWith("'")) {
      cleaned = cleaned.substring(1, cleaned.length - 1);
    }
    const commentIdx = cleaned.indexOf('#');
    if (commentIdx !== -1) {
      cleaned = cleaned.substring(0, commentIdx).trim();
    }
    return cleaned.trim();
  };

  for (const line of lines) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith('#')) continue;

    const eqIndex = trimmed.indexOf('=');
    if (eqIndex <= 0) continue;

    const key = trimmed.substring(0, eqIndex).trim();
    const val = stripQuotesAndComments(trimmed.substring(eqIndex + 1));

    if (key === 'APP_PROXY_ENABLE') {
      enabled = val === '1';
    } else if (key === 'APP_PROXY_MODE') {
      mode = val === 'whitelist' ? 'whitelist' : 'blacklist';
    } else if (key === 'PROXY_APPS_LIST') {
      proxyList = val.split(/\s+/).filter(Boolean);
    } else if (key === 'BYPASS_APPS_LIST') {
      bypassList = val.split(/\s+/).filter(Boolean);
    }
  }

  const proxiedAppItems = mode === 'whitelist' ? proxyList : bypassList;

  return {
    appProxyEnabled: enabled,
    appProxyMode: mode,
    proxiedAppItems
  };
};

/**
 * 将应用加入当前模式对应的列表（whitelist→代理列表，blacklist→绕过列表）。
 * @param packageName  包名
 * @param userId  用户 ID（默认 '0'，写入 userId:packageName 形式）
 */
export const addProxyApp = async (packageName: string, userId: string = '0'): Promise<void> => {
  if (!isKsuEnv()) {
    const storedChecked = localStorage.getItem('mock_checked_apps');
    const currentApps = storedChecked ? JSON.parse(storedChecked) as string[] : [];
    if (!currentApps.includes(packageName)) {
      currentApps.push(packageName);
      localStorage.setItem('mock_checked_apps', JSON.stringify(currentApps));
    }
    return;
  }
  const state = await getEbpfConfigState();
  const currentApps = state.proxiedAppItems;
  const newItem = `${userId}:${packageName}`;
  if (currentApps.includes(newItem)) return;

  const listKey = state.appProxyMode === 'blacklist' ? 'BYPASS_APPS_LIST' : 'PROXY_APPS_LIST';
  const newList = currentApps.length === 0 ? newItem : [...currentApps, newItem].join(' ');
  await writeEbpfValue(listKey, newList, true);
};

/**
 * 将应用从代理/绕过列表中移除（兼容 userId:packageName 与裸 packageName）。
 * @param packageName  包名
 * @param userId  用户 ID（默认 '0'）
 */
export const removeProxyApp = async (packageName: string, userId: string = '0'): Promise<void> => {
  if (!isKsuEnv()) {
    const storedChecked = localStorage.getItem('mock_checked_apps');
    const currentApps = storedChecked ? JSON.parse(storedChecked) as string[] : [];
    const newList = currentApps.filter(item => item !== packageName);
    localStorage.setItem('mock_checked_apps', JSON.stringify(newList));
    return;
  }
  const state = await getEbpfConfigState();
  const currentApps = state.proxiedAppItems;
  const target = `${userId}:${packageName}`;
  if (!currentApps.includes(target) && !currentApps.includes(packageName)) return;

  const listKey = state.appProxyMode === 'blacklist' ? 'BYPASS_APPS_LIST' : 'PROXY_APPS_LIST';
  const newList = currentApps.filter(item => item !== target && item !== packageName).join(' ');
  await writeEbpfValue(listKey, newList, true);
};

// ===================================================================
// 七、KernelSU 原生 UI / 系统能力
// ===================================================================

/**
 * 显示 Android 原生 toast。非真机环境降级为 alert。
 * @param message  提示文本
 */
export const showToast = (message: string): void => {
  if (!isKsuEnv()) {
    console.log(`[KSU Mock Toast]: ${message}`);
    alert(message);
    return;
  }
  try {
    toast(message);
  } catch (err) {
    console.error('Toast failed:', err);
  }
};

/**
 * 请求 WebView 进入/退出全屏。
 * @param enable  true=全屏
 */
export const setFullScreen = (enable: boolean): void => {
  if (!isKsuEnv()) {
    console.log(`[KSU Mock FullScreen]: ${enable}`);
    return;
  }
  try {
    fullScreen(enable);
  } catch (err) {
    console.error('Fullscreen API failed:', err);
  }
};

/**
 * 设置 edge-to-edge 沉浸式布局（配合 internal/insets.css 使用）。
 * @param enable  true=启用
 */
export const setEdgeToEdge = (enable: boolean): void => {
  if (!isKsuEnv()) {
    console.log(`[KSU Mock EdgeToEdge]: ${enable}`);
    return;
  }
  try {
    enableEdgeToEdge(enable);
  } catch (err) {
    console.error('EdgeToEdge API failed:', err);
  }
};

/**
 * 获取当前模块信息（KernelSU moduleInfo）。
 * @returns 解析后的模块信息对象；失败或非真机返回 null
 */
export const getModuleInfo = (): Record<string, any> | null => {
  if (!isKsuEnv()) {
    console.log('[KSU Mock moduleInfo]');
    return null;
  }
  try {
    const raw = moduleInfo();
    return raw ? JSON.parse(raw) : null;
  } catch (err) {
    console.error('moduleInfo failed:', err);
    return null;
  }
};

/**
 * 退出当前 WebUI Activity（返回 KernelSU 管理器）。
 * 非真机环境为空操作。
 */
export const exitWebUI = (): void => {
  if (!isKsuEnv()) {
    console.log('[KSU Mock exit]');
    return;
  }
  try {
    exit();
  } catch (err) {
    console.error('exit failed:', err);
  }
};

/**
 * 用系统默认应用打开外部链接（浏览器 / Telegram 等）。
 * WebUI 运行在 KernelSU 的 WebView 中，`<a target="_blank">` / window.open 无法
 * 唤起系统应用，故走 root shell 发一个 Android VIEW Intent（am start）。
 * 非真机环境降级为 window.open。
 * @param url  目标 URL（http/https 或自定义 scheme）
 */
export const openExternalUrl = async (url: string): Promise<void> => {
  if (!isKsuEnv()) {
    console.log(`[KSU Mock openExternalUrl]: ${url}`);
    window.open(url, '_blank');
    return;
  }
  // 单引号包裹 URL 防止 shell 解析特殊字符；URL 内的单引号转义为 '\''
  const safeUrl = url.replace(/'/g, `'\\''`);
  const cmd = `am start -a android.intent.action.VIEW -d '${safeUrl}'`;
  try {
    const res = await execAsync(cmd);
    if (res.errno !== 0) {
      showToast('无法打开链接');
      console.error('openExternalUrl failed:', res.stderr);
    }
  } catch (err) {
    showToast('无法打开链接');
    console.error('openExternalUrl error:', err);
  }
};
