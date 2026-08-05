/**
 * WebUI 平台桥接层。模块与代理状态统一由 ModuleClient 获取；此处只保留
 * KernelSU 原生能力、临时文件写入和 Android 应用信息查询。
 */
import {
  exec,
  toast,
  enableEdgeToEdge,
  moduleInfo,
  listPackages,
  getPackagesInfo
} from 'kernelsu';

export interface ExecResults {
  errno: number;
  stdout: string;
  stderr: string;
}

export interface ExecOptions {
  cwd?: string;
  env?: Record<string, string>;
}

export interface PackagesInfo {
  packageName: string;
  versionName: string;
  versionCode: number;
  appLabel: string;
  isSystem: boolean;
  uid: number;
}

const ANDROID_UID_PER_USER = 100000;

export const isKsuEnv = (): boolean =>
  typeof (window as any).ksu !== 'undefined' || typeof (window as any).KSU !== 'undefined';

export const execAsync = async (cmd: string, options?: ExecOptions): Promise<ExecResults> => {
  if (!isKsuEnv()) {
    console.warn(`[KSU Mock execAsync] ${cmd}`);
    return { errno: 0, stdout: '', stderr: '' };
  }
  try {
    return await exec(cmd, options ?? {});
  } catch (error: any) {
    return { errno: -1, stdout: '', stderr: error?.message || String(error) };
  }
};

/** 写入仅供 netproxyctl 消费的临时文件。 */
export const writeFileContent = async (path: string, content: string): Promise<void> => {
  if (!isKsuEnv()) {
    localStorage.setItem(`mock_file_${path}`, content);
    return;
  }

  const bytes = new TextEncoder().encode(content);
  let binary = '';
  for (const byte of bytes) binary += String.fromCharCode(byte);
  const encoded = btoa(binary);
  const quotedPath = `'${path.replace(/'/g, `'"'"'`)}'`;
  const result = await execAsync(`printf '%s' '${encoded}' | base64 -d > ${quotedPath}`);
  if (result.errno !== 0) {
    throw new Error(result.stderr || `无法写入临时文件: ${path}`);
  }
};

const normalizePackagesInfo = (infos: PackagesInfo[]): PackagesInfo[] => {
  const unique = new Map<string, PackagesInfo>();
  for (const info of infos) {
    if (!info || typeof info.packageName !== 'string' || !Number.isInteger(info.uid) || info.uid < 0) continue;
    const packageName = info.packageName.trim();
    if (!packageName) continue;
    const userId = Math.floor(info.uid / ANDROID_UID_PER_USER);
    const key = `${userId}:${packageName}`;
    if (!unique.has(key)) unique.set(key, { ...info, packageName });
  }
  return Array.from(unique.values());
};

export const getAppIconUrl = (packageName: string): string => `ksu://icon/${packageName}`;

export const getAppPackagesList = async (
  filter: 'user' | 'system' | 'all' = 'all'
): Promise<PackagesInfo[]> => {
  if (!isKsuEnv()) {
    return [
      { packageName: 'com.google.android.youtube', versionName: '19.01.01', versionCode: 12345, appLabel: 'YouTube', isSystem: true, uid: 10123 },
      { packageName: 'com.twitter.android', versionName: '10.2.0', versionCode: 67890, appLabel: 'Twitter/X', isSystem: false, uid: 10456 },
      { packageName: 'com.tencent.mm', versionName: '8.0.48', versionCode: 11223, appLabel: '微信', isSystem: false, uid: 10888 },
      { packageName: 'com.eg.android.Alipay', versionName: '10.5.80', versionCode: 44556, appLabel: '支付宝', isSystem: false, uid: 10999 },
      { packageName: 'com.android.settings', versionName: '16', versionCode: 99999, appLabel: '设置', isSystem: true, uid: 10000 }
    ].filter(app => filter === 'all' || (filter === 'system' ? app.isSystem : !app.isSystem));
  }

  try {
    const names = listPackages(filter);
    if (!names?.length) return [];
    return normalizePackagesInfo((getPackagesInfo(names) ?? []) as PackagesInfo[]);
  } catch (error) {
    console.error('读取 Android 应用列表失败:', error);
    return [];
  }
};

export const showToast = (message: string): void => {
  if (!isKsuEnv()) {
    console.log(`[KSU Mock Toast] ${message}`);
    return;
  }
  try {
    toast(message);
  } catch (error) {
    console.error('显示 Toast 失败:', error);
  }
};

export const setEdgeToEdge = (enabled: boolean): void => {
  if (!isKsuEnv()) return;
  try {
    enableEdgeToEdge(enabled);
  } catch (error) {
    console.error('设置沉浸式布局失败:', error);
  }
};

export const getModuleInfo = (): Record<string, any> | null => {
  if (!isKsuEnv()) return null;
  try {
    const raw = moduleInfo();
    return raw ? JSON.parse(raw) : null;
  } catch (error) {
    console.error('读取模块信息失败:', error);
    return null;
  }
};

export const openExternalUrl = async (url: string): Promise<void> => {
  if (!isKsuEnv()) {
    window.open(url, '_blank');
    return;
  }
  const safeUrl = url.replace(/'/g, `'\\''`);
  const result = await execAsync(`am start -a android.intent.action.VIEW -d '${safeUrl}'`);
  if (result.errno !== 0) {
    showToast('无法打开链接');
    console.error('打开外部链接失败:', result.stderr);
  }
};
