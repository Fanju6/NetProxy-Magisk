<script setup lang="ts">
/**
 * @file SettingsLayout.vue
 * @description 设置页共享控制器：统一加载与写入 module.conf、ebpf.conf，
 *   并向设置子页提供状态、编辑动作和 eBPF 配置应用入口。
 */
import { ref, provide, onMounted } from 'vue';
import { useI18n } from 'vue-i18n';
import { showToast, readFileContent, writeFileContent, isKsuEnv, runCli } from '../utils/ksu';
import { useBackDismiss } from '../composables/useBackDismiss';

const { t } = useI18n();

const MODULE_CONF = '/data/adb/modules/netproxy/config/module.conf';
const EBPF_CONF = '/data/adb/modules/netproxy/config/ebpf/ebpf.conf';

/** 设置页聚合状态：模块设置、eBPF 入站和 Wi-Fi 自动切换。 */
export interface SettingsState {
  autoStartEnabled: boolean;
  selectorUrlTestEnabled: boolean;
  gmsFixEnabled: boolean;
  network: string;
  udpTimeout: string;
  dnsMode: 'hijack' | 'off';
  cgroupPath: string;
  ipv6Enabled: boolean;
  bypassRuleSets: string;
  sharedNetworkEnabled: boolean;
  sharedInterfaces: string;
  tcpMapCapacity: string;
  udpMapCapacity: string;
  socketMapCapacity: string;
  sharedMapCapacity: string;
  wifiAutoSwitch: boolean;
  wifiSsidMode: 'blacklist' | 'whitelist';
  wifiSsidList: string;
  proxyOnCellular: boolean;
  wifiInterface: string;
}

type SettingsKey = keyof SettingsState;

const settingsState = ref<SettingsState>({
  autoStartEnabled: false,
  selectorUrlTestEnabled: true,
  gmsFixEnabled: false,
  network: '',
  udpTimeout: '5m',
  dnsMode: 'hijack',
  cgroupPath: '',
  ipv6Enabled: true,
  bypassRuleSets: 'direct ChinaIP',
  sharedNetworkEnabled: false,
  sharedInterfaces: 'wlan2',
  tcpMapCapacity: '65536',
  udpMapCapacity: '65536',
  socketMapCapacity: '65536',
  sharedMapCapacity: '65536',
  wifiAutoSwitch: false,
  wifiSsidMode: 'blacklist',
  wifiSsidList: '',
  proxyOnCellular: true,
  wifiInterface: 'wlan0'
});

const ebpfFieldMap: Partial<Record<SettingsKey, string>> = {
  network: 'EBPF_NETWORK',
  udpTimeout: 'EBPF_UDP_TIMEOUT',
  dnsMode: 'EBPF_DNS_MODE',
  cgroupPath: 'EBPF_CGROUP_PATH',
  ipv6Enabled: 'EBPF_IPV6',
  bypassRuleSets: 'EBPF_BYPASS_RULE_SETS',
  sharedNetworkEnabled: 'EBPF_SHARED_NETWORK',
  sharedInterfaces: 'EBPF_SHARED_INTERFACES',
  tcpMapCapacity: 'EBPF_TCP_MAP_CAPACITY',
  udpMapCapacity: 'EBPF_UDP_MAP_CAPACITY',
  socketMapCapacity: 'EBPF_SOCKET_MAP_CAPACITY',
  sharedMapCapacity: 'EBPF_SHARED_MAP_CAPACITY'
};

const moduleFieldMap: Partial<Record<SettingsKey, string>> = {
  wifiAutoSwitch: 'WIFI_AUTO_SWITCH',
  wifiSsidMode: 'WIFI_SSID_MODE',
  wifiSsidList: 'WIFI_SSID_LIST',
  proxyOnCellular: 'PROXY_ON_CELLULAR',
  wifiInterface: 'WIFI_INTERFACE'
};

const quotedFields = new Set<SettingsKey>([
  'network',
  'udpTimeout',
  'dnsMode',
  'cgroupPath',
  'bypassRuleSets',
  'sharedInterfaces',
  'wifiSsidMode',
  'wifiSsidList',
  'wifiInterface'
]);

const showEditDialog = ref(false);
const editKey = ref<SettingsKey>('udpTimeout');
const editTitle = ref('');
const editLabel = ref('');
const editValue = ref('');
const editType = ref<'text' | 'number'>('text');
const isApplying = ref(false);

useBackDismiss(
  () => showEditDialog.value,
  () => { showEditDialog.value = false; }
);

// ===================================================================
// 配置文件解析与写入
// ===================================================================

const parseConfigFile = (content: string): Record<string, string> => {
  const result: Record<string, string> = {};
  for (const line of content.replace(/\r\n/g, '\n').split('\n')) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith('#')) continue;
    const eqIndex = trimmed.indexOf('=');
    if (eqIndex < 1) continue;

    const key = trimmed.substring(0, eqIndex).trim();
    let value = trimmed.substring(eqIndex + 1).trim();
    if ((value.startsWith('"') && value.endsWith('"')) || (value.startsWith("'") && value.endsWith("'"))) {
      value = value.substring(1, value.length - 1);
    }
    result[key] = value;
  }
  return result;
};

const writeConfigValue = (content: string, key: string, value: string, forceQuotes = false): string => {
  const lines = content.replace(/\r\n/g, '\n').split('\n');
  const escaped = value
    .replace(/\\/g, '\\\\')
    .replace(/"/g, '\\"')
    .replace(/\$/g, '\\$')
    .replace(/`/g, '\\`');
  const formatted = forceQuotes ? `"${escaped}"` : value;
  let found = false;

  const updated = lines.map(line => {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith('#')) return line;
    const eqIndex = trimmed.indexOf('=');
    if (eqIndex < 1 || trimmed.substring(0, eqIndex).trim() !== key) return line;
    found = true;
    return `${key}=${formatted}`;
  });

  if (!found) updated.push(`${key}=${formatted}`);
  return updated.join('\n');
};

const getValue = (config: Record<string, string>, key: string, fallback: string): string =>
  config[key] === undefined ? fallback : config[key];

const getBool = (config: Record<string, string>, key: string, fallback: boolean): boolean =>
  config[key] === undefined ? fallback : config[key] === '1';

const loadSettings = async () => {
  try {
    const [moduleContent, ebpfContent] = await Promise.all([
      readFileContent(MODULE_CONF),
      readFileContent(EBPF_CONF)
    ]);
    const moduleConfig = parseConfigFile(moduleContent);
    const ebpfConfig = parseConfigFile(ebpfContent);

    settingsState.value.autoStartEnabled = getBool(moduleConfig, 'AUTO_START', false);
    settingsState.value.selectorUrlTestEnabled = getValue(moduleConfig, 'SELECTOR_MODE', 'urltest') !== 'manual';
    settingsState.value.gmsFixEnabled = getBool(moduleConfig, 'GMS_FIX', false);
    settingsState.value.wifiAutoSwitch = getBool(moduleConfig, 'WIFI_AUTO_SWITCH', false);
    settingsState.value.wifiSsidMode = getValue(moduleConfig, 'WIFI_SSID_MODE', 'blacklist') === 'whitelist' ? 'whitelist' : 'blacklist';
    settingsState.value.wifiSsidList = getValue(moduleConfig, 'WIFI_SSID_LIST', '');
    settingsState.value.proxyOnCellular = getBool(moduleConfig, 'PROXY_ON_CELLULAR', true);
    settingsState.value.wifiInterface = getValue(moduleConfig, 'WIFI_INTERFACE', 'wlan0');

    settingsState.value.network = getValue(ebpfConfig, 'EBPF_NETWORK', '');
    settingsState.value.udpTimeout = getValue(ebpfConfig, 'EBPF_UDP_TIMEOUT', '5m');
    settingsState.value.dnsMode = getValue(ebpfConfig, 'EBPF_DNS_MODE', 'hijack') === 'off' ? 'off' : 'hijack';
    settingsState.value.cgroupPath = getValue(ebpfConfig, 'EBPF_CGROUP_PATH', '');
    settingsState.value.ipv6Enabled = getBool(ebpfConfig, 'EBPF_IPV6', true);
    settingsState.value.bypassRuleSets = getValue(ebpfConfig, 'EBPF_BYPASS_RULE_SETS', 'direct ChinaIP');
    settingsState.value.sharedNetworkEnabled = getBool(ebpfConfig, 'EBPF_SHARED_NETWORK', false);
    settingsState.value.sharedInterfaces = getValue(ebpfConfig, 'EBPF_SHARED_INTERFACES', 'wlan2');
    settingsState.value.tcpMapCapacity = getValue(ebpfConfig, 'EBPF_TCP_MAP_CAPACITY', '65536');
    settingsState.value.udpMapCapacity = getValue(ebpfConfig, 'EBPF_UDP_MAP_CAPACITY', '65536');
    settingsState.value.socketMapCapacity = getValue(ebpfConfig, 'EBPF_SOCKET_MAP_CAPACITY', '65536');
    settingsState.value.sharedMapCapacity = getValue(ebpfConfig, 'EBPF_SHARED_MAP_CAPACITY', '65536');
  } catch (error) {
    console.warn('Failed to load native configs, using mock persistence:', error);
    for (const key of Object.keys(settingsState.value) as SettingsKey[]) {
      const stored = localStorage.getItem(`mock_settings_${key}`);
      if (stored === null) continue;
      const current = settingsState.value[key];
      (settingsState.value as any)[key] = typeof current === 'boolean' ? stored === 'true' : stored;
    }
  }
};

const writeSetting = async (
  path: string,
  configKey: string,
  value: string,
  forceQuotes: boolean,
  stateKey: SettingsKey
) => {
  if (!isKsuEnv()) {
    localStorage.setItem(`mock_settings_${stateKey}`, String(settingsState.value[stateKey]));
    return;
  }

  const content = await readFileContent(path);
  await writeFileContent(path, writeConfigValue(content, configKey, value, forceQuotes));
};

const setEbpfValue = async (key: SettingsKey, value: string | boolean) => {
  const configKey = ebpfFieldMap[key];
  if (!configKey) return;
  const previous = settingsState.value[key];
  (settingsState.value as any)[key] = value;
  try {
    await writeSetting(EBPF_CONF, configKey, typeof value === 'boolean' ? (value ? '1' : '0') : value, quotedFields.has(key), key);
    showToast(t('settings.prefUpdated'));
  } catch (error) {
    (settingsState.value as any)[key] = previous;
    console.error(`Failed to update ${configKey} in ebpf.conf:`, error);
    showToast(t('settings.updateEbpfFailed'));
  }
};

const setModuleValue = async (key: SettingsKey, value: string | boolean) => {
  const configKey = moduleFieldMap[key];
  if (!configKey) return;
  const previous = settingsState.value[key];
  (settingsState.value as any)[key] = value;
  try {
    await writeSetting(MODULE_CONF, configKey, typeof value === 'boolean' ? (value ? '1' : '0') : value, quotedFields.has(key), key);
    showToast(t('settings.prefUpdated'));
  } catch (error) {
    (settingsState.value as any)[key] = previous;
    console.error(`Failed to update ${configKey} in module.conf:`, error);
    showToast(t('settings.updateModuleFailed'));
  }
};

const updateGlobalModuleSetting = async (key: string, value: string, stateKey: SettingsKey) => {
  try {
    await writeSetting(MODULE_CONF, key, value, false, stateKey);
  } catch (error) {
    console.error(`Failed to update ${key} in module.conf:`, error);
    showToast(t('settings.updateModuleFailed'));
  }
};

// ===================================================================
// 设置动作
// ===================================================================

const toggleAutoStart = async () => {
  const value = !settingsState.value.autoStartEnabled;
  settingsState.value.autoStartEnabled = value;
  await updateGlobalModuleSetting('AUTO_START', value ? '1' : '0', 'autoStartEnabled');
  showToast(value ? t('settings.autoStartOn') : t('settings.autoStartOff'));
};

const toggleSelectorUrlTest = async () => {
  const value = !settingsState.value.selectorUrlTestEnabled;
  settingsState.value.selectorUrlTestEnabled = value;
  await updateGlobalModuleSetting('SELECTOR_MODE', value ? 'urltest' : 'manual', 'selectorUrlTestEnabled');
  showToast(value ? t('settings.urltestOn') : t('settings.urltestOff'));
};

const toggleGmsFix = async () => {
  const value = !settingsState.value.gmsFixEnabled;
  settingsState.value.gmsFixEnabled = value;
  await updateGlobalModuleSetting('GMS_FIX', value ? '1' : '0', 'gmsFixEnabled');
  showToast(value ? t('settings.gmsFixOn') : t('settings.gmsFixOff'));
};

const toggleEbpfBool = async (key: SettingsKey) => {
  await setEbpfValue(key, !Boolean(settingsState.value[key]));
};

const toggleModuleBool = async (key: SettingsKey) => {
  await setModuleValue(key, !Boolean(settingsState.value[key]));
};

const openEditPreference = (
  key: SettingsKey,
  title: string,
  label: string,
  type: 'text' | 'number' = 'text'
) => {
  editKey.value = key;
  editTitle.value = title;
  editLabel.value = label;
  editValue.value = String(settingsState.value[key]);
  editType.value = type;
  showEditDialog.value = true;
};

const handleSaveEdit = async () => {
  const key = editKey.value;
  const value = editValue.value.trim();
  if (editType.value === 'number' && (!/^\d+$/.test(value) || Number(value) < 1)) {
    showToast(t('settings.invalidNumber'));
    return;
  }
  if (ebpfFieldMap[key]) {
    await setEbpfValue(key, value);
  } else if (moduleFieldMap[key]) {
    await setModuleValue(key, value);
  }
  showEditDialog.value = false;
};

const applyEbpfConfig = async () => {
  if (isApplying.value) return;
  isApplying.value = true;
  try {
    await runCli('service restart', { detach: true });
    showToast(t('proxy.applySuccess'));
  } catch (error: any) {
    console.error('Failed to apply eBPF config:', error);
    showToast(t('proxy.applyFailed', { msg: error?.message || String(error) }));
  } finally {
    isApplying.value = false;
  }
};

onMounted(loadSettings);

provide('settingsState', settingsState);
provide('toggleAutoStart', toggleAutoStart);
provide('toggleSelectorUrlTest', toggleSelectorUrlTest);
provide('toggleGmsFix', toggleGmsFix);
provide('toggleEbpfBool', toggleEbpfBool);
provide('toggleModuleBool', toggleModuleBool);
provide('setEbpfValue', setEbpfValue);
provide('setModuleValue', setModuleValue);
provide('openEditPreference', openEditPreference);
provide('applyEbpfConfig', applyEbpfConfig);
provide('isApplyingEbpfConfig', isApplying);
</script>

<template>
  <div class="settings-layout-wrapper">
    <router-view v-slot="{ Component }">
      <component :is="Component" />
    </router-view>

    <md-dialog :open="showEditDialog" @close="showEditDialog = false" class="transparent-scrim">
      <div slot="headline">{{ editTitle }}</div>
      <div slot="content" class="display-dialog-content">
        <md-outlined-text-field
          class="edit-text-field"
          :label="editLabel"
          :type="editType"
          v-model="editValue"
          autofocus>
        </md-outlined-text-field>
      </div>
      <div slot="actions">
        <md-text-button @click="showEditDialog = false">{{ t('common.cancel') }}</md-text-button>
        <md-text-button @click="handleSaveEdit">{{ t('common.save') }}</md-text-button>
      </div>
    </md-dialog>
  </div>
</template>

<style scoped>
.settings-layout-wrapper {
  width: 100%;
  height: 100%;
}

.display-dialog-content {
  display: flex;
  flex-direction: column;
  gap: 12px;
  margin-top: 8px;
}

.edit-text-field {
  width: 100%;
}

.transparent-scrim {
  --md-dialog-scrim-color: rgba(0, 0, 0, 0.4);
}
</style>
