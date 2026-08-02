<script setup lang="ts">
/**
 * @file ProxySettingsScreen.vue
 * @description eBPF 透明代理设置页。配置写入 ebpf.conf 与 module.conf，
 *   用户确认后统一重启服务，避免每次修改都重建 eBPF 入站。
 */
import { inject } from 'vue';
import type { Ref } from 'vue';
import { useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import type { SettingsState } from './SettingsLayout.vue';

type SettingsKey = keyof SettingsState;

const router = useRouter();
const { t } = useI18n();
const settingsState = inject<Ref<SettingsState>>('settingsState')!;
const toggleEbpfBool = inject<(key: SettingsKey) => Promise<void>>('toggleEbpfBool')!;
const toggleModuleBool = inject<(key: SettingsKey) => Promise<void>>('toggleModuleBool')!;
const setEbpfValue = inject<(key: SettingsKey, value: string | boolean) => Promise<void>>('setEbpfValue')!;
const setModuleValue = inject<(key: SettingsKey, value: string | boolean) => Promise<void>>('setModuleValue')!;
const openEditPreference = inject<(
  key: SettingsKey,
  title: string,
  label: string,
  type?: 'text' | 'number'
) => void>('openEditPreference')!;
const applyEbpfConfig = inject<() => Promise<void>>('applyEbpfConfig')!;
const isApplying = inject<Ref<boolean>>('isApplyingEbpfConfig')!;

const handleEbpfSelect = (key: SettingsKey, event: Event) => {
  void setEbpfValue(key, (event.target as HTMLSelectElement).value);
};

const handleModuleSelect = (key: SettingsKey, event: Event) => {
  void setModuleValue(key, (event.target as HTMLSelectElement).value);
};
</script>

<template>
  <Teleport to="body">
    <div class="sub-screen-overlay scroll-container">
      <header class="sub-top-bar">
        <div class="sub-top-bar-left">
          <md-icon-button @click="router.back()" class="sub-back-btn">
            <md-icon>
              <svg viewBox="0 0 24 24">
                <path d="M20 11H7.83l5.59-5.59L12 4l-8 8 8 8 1.41-1.41L7.83 13H20v-2z" fill="currentColor"/>
              </svg>
            </md-icon>
          </md-icon-button>
          <h1 class="sub-screen-title">{{ t('proxy.title') }}</h1>
        </div>
      </header>

      <div class="sub-screen-content">
        <div class="settings-lazy-column">
          <div class="small-title">{{ t('proxy.coreConfig') }}</div>
          <div class="config-card">
            <div class="dropdown-pref-row">
              <div class="pref-text">
                <span class="pref-title">{{ t('proxy.network') }}</span>
              </div>
              <select :value="settingsState.network" @change="handleEbpfSelect('network', $event)" class="pref-dropdown">
                <option value="">{{ t('proxy.networkAll') }}</option>
                <option value="tcp">{{ t('proxy.networkTcp') }}</option>
                <option value="udp">{{ t('proxy.networkUdp') }}</option>
              </select>
            </div>

            <div class="pref-inner-divider"></div>

            <div class="switch-pref-row" @click="setEbpfValue('dnsMode', settingsState.dnsMode === 'hijack' ? 'off' : 'hijack')">
              <div class="pref-text">
                <span class="pref-title">{{ t('proxy.dnsHijack') }}</span>
                <span class="pref-summary">{{ t('proxy.dnsHijackDesc') }}</span>
              </div>
              <md-switch
                icons
                :selected="settingsState.dnsMode === 'hijack'"
                @click.stop="setEbpfValue('dnsMode', settingsState.dnsMode === 'hijack' ? 'off' : 'hijack')">
              </md-switch>
            </div>

            <div class="pref-inner-divider"></div>

            <div class="switch-pref-row" @click="toggleEbpfBool('ipv6Enabled')">
              <div class="pref-text">
                <span class="pref-title">{{ t('proxy.ipv6') }}</span>
                <span class="pref-summary">{{ t('proxy.ipv6Desc') }}</span>
              </div>
              <md-switch icons :selected="settingsState.ipv6Enabled" @click.stop="toggleEbpfBool('ipv6Enabled')"></md-switch>
            </div>

            <div class="pref-inner-divider"></div>

            <div class="arrow-pref-row" @click="openEditPreference('udpTimeout', t('proxy.udpTimeout'), t('proxy.udpTimeoutLabel'))">
              <div class="pref-text">
                <span class="pref-title">{{ t('proxy.udpTimeout') }}</span>
                <span class="pref-summary">{{ settingsState.udpTimeout }}</span>
              </div>
            </div>

            <div class="pref-inner-divider"></div>

            <div class="arrow-pref-row" @click="openEditPreference('cgroupPath', t('proxy.cgroupPath'), t('proxy.cgroupPathLabel'))">
              <div class="pref-text">
                <span class="pref-title">{{ t('proxy.cgroupPath') }}</span>
                <span class="pref-summary text-ellipsis">{{ settingsState.cgroupPath || t('proxy.cgroupAuto') }}</span>
              </div>
            </div>
          </div>

          <div class="small-title">{{ t('proxy.bypassConfig') }}</div>
          <div class="config-card">
            <div class="arrow-pref-row" @click="openEditPreference('bypassRuleSets', t('proxy.bypassRuleSets'), t('proxy.bypassRuleSetsLabel'))">
              <div class="pref-text">
                <span class="pref-title">{{ t('proxy.bypassRuleSets') }}</span>
                <span class="pref-summary text-ellipsis">{{ settingsState.bypassRuleSets || t('common.notSet') }}</span>
              </div>
            </div>
          </div>

          <div class="small-title">{{ t('proxy.sharedConfig') }}</div>
          <div class="config-card">
            <div class="switch-pref-row" @click="toggleEbpfBool('sharedNetworkEnabled')">
              <div class="pref-text">
                <span class="pref-title">{{ t('proxy.sharedEnable') }}</span>
                <span class="pref-summary">{{ t('proxy.sharedEnableDesc') }}</span>
              </div>
              <md-switch
                icons
                :selected="settingsState.sharedNetworkEnabled"
                @click.stop="toggleEbpfBool('sharedNetworkEnabled')">
              </md-switch>
            </div>

            <div class="pref-inner-divider"></div>

            <div class="arrow-pref-row" @click="openEditPreference('sharedInterfaces', t('proxy.sharedInterfaces'), t('proxy.sharedInterfacesLabel'))">
              <div class="pref-text">
                <span class="pref-title">{{ t('proxy.sharedInterfaces') }}</span>
                <span class="pref-summary text-ellipsis">{{ settingsState.sharedInterfaces || t('common.notSet') }}</span>
              </div>
            </div>
          </div>

          <div class="small-title">{{ t('proxy.mapConfig') }}</div>
          <div class="config-card">
            <div class="arrow-pref-row" @click="openEditPreference('tcpMapCapacity', t('proxy.tcpMap'), t('proxy.mapCapacityLabel'), 'number')">
              <div class="pref-text">
                <span class="pref-title">{{ t('proxy.tcpMap') }}</span>
                <span class="pref-summary">{{ settingsState.tcpMapCapacity }}</span>
              </div>
            </div>
            <div class="pref-inner-divider"></div>
            <div class="arrow-pref-row" @click="openEditPreference('udpMapCapacity', t('proxy.udpMap'), t('proxy.mapCapacityLabel'), 'number')">
              <div class="pref-text">
                <span class="pref-title">{{ t('proxy.udpMap') }}</span>
                <span class="pref-summary">{{ settingsState.udpMapCapacity }}</span>
              </div>
            </div>
            <div class="pref-inner-divider"></div>
            <div class="arrow-pref-row" @click="openEditPreference('socketMapCapacity', t('proxy.socketMap'), t('proxy.mapCapacityLabel'), 'number')">
              <div class="pref-text">
                <span class="pref-title">{{ t('proxy.socketMap') }}</span>
                <span class="pref-summary">{{ settingsState.socketMapCapacity }}</span>
              </div>
            </div>
            <div class="pref-inner-divider"></div>
            <div class="arrow-pref-row" @click="openEditPreference('sharedMapCapacity', t('proxy.sharedMap'), t('proxy.mapCapacityLabel'), 'number')">
              <div class="pref-text">
                <span class="pref-title">{{ t('proxy.sharedMap') }}</span>
                <span class="pref-summary">{{ settingsState.sharedMapCapacity }}</span>
              </div>
            </div>
          </div>

          <div class="small-title">{{ t('proxy.wifiConfig') }}</div>
          <div class="config-card">
            <div class="switch-pref-row" @click="toggleModuleBool('wifiAutoSwitch')">
              <div class="pref-text">
                <span class="pref-title">{{ t('proxy.wifiAutoSwitch') }}</span>
                <span class="pref-summary">{{ t('proxy.wifiAutoSwitchDesc') }}</span>
              </div>
              <md-switch icons :selected="settingsState.wifiAutoSwitch" @click.stop="toggleModuleBool('wifiAutoSwitch')"></md-switch>
            </div>
            <div class="pref-inner-divider"></div>
            <div class="dropdown-pref-row">
              <div class="pref-text"><span class="pref-title">{{ t('proxy.wifiSsidMode') }}</span></div>
              <select :value="settingsState.wifiSsidMode" @change="handleModuleSelect('wifiSsidMode', $event)" class="pref-dropdown">
                <option value="blacklist">{{ t('proxy.wifiBlacklist') }}</option>
                <option value="whitelist">{{ t('proxy.wifiWhitelist') }}</option>
              </select>
            </div>
            <div class="pref-inner-divider"></div>
            <div class="arrow-pref-row" @click="openEditPreference('wifiSsidList', t('proxy.wifiSsidList'), t('proxy.wifiSsidListLabel'))">
              <div class="pref-text">
                <span class="pref-title">{{ t('proxy.wifiSsidList') }}</span>
                <span class="pref-summary text-ellipsis">{{ settingsState.wifiSsidList || t('common.notSet') }}</span>
              </div>
            </div>
            <div class="pref-inner-divider"></div>
            <div class="switch-pref-row" @click="toggleModuleBool('proxyOnCellular')">
              <div class="pref-text">
                <span class="pref-title">{{ t('proxy.proxyOnCellular') }}</span>
                <span class="pref-summary">{{ t('proxy.proxyOnCellularDesc') }}</span>
              </div>
              <md-switch icons :selected="settingsState.proxyOnCellular" @click.stop="toggleModuleBool('proxyOnCellular')"></md-switch>
            </div>
            <div class="pref-inner-divider"></div>
            <div class="arrow-pref-row" @click="openEditPreference('wifiInterface', t('proxy.wifiInterface'), t('proxy.wifiInterfaceLabel'))">
              <div class="pref-text">
                <span class="pref-title">{{ t('proxy.wifiInterface') }}</span>
                <span class="pref-summary">{{ settingsState.wifiInterface }}</span>
              </div>
            </div>
          </div>

          <div class="config-card apply-card">
            <div class="pref-text">
              <span class="pref-title">{{ t('proxy.applyConfig') }}</span>
              <span class="pref-summary">{{ t('proxy.applyConfigDesc') }}</span>
            </div>
            <md-filled-button :disabled="isApplying" @click="applyEbpfConfig">
              {{ isApplying ? t('proxy.applying') : t('proxy.applyConfig') }}
            </md-filled-button>
          </div>

          <div class="bottom-padding-spacer"></div>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.sub-screen-overlay {
  position: fixed;
  inset: 0;
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
  from { opacity: 0; transform: translateY(30px); }
  to { opacity: 1; transform: translateY(0); }
}

.sub-top-bar {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: calc(64px + var(--top-inset));
  padding: var(--top-inset) 16px 0;
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

.sub-screen-content {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  overflow-x: hidden;
  padding: 16px calc(16px + env(safe-area-inset-right)) calc(24px + var(--bottom-inset)) calc(16px + env(safe-area-inset-left));
  box-sizing: border-box;
  width: 100%;
}

.settings-lazy-column {
  display: flex;
  flex-direction: column;
  gap: 12px;
  width: 100%;
  max-width: 800px;
  margin: 0 auto;
}

.config-card {
  padding: 0;
  gap: 0;
  overflow: hidden;
  background-color: var(--md-sys-color-surface-container);
  border-radius: var(--radius-xl);
  border: none;
  display: flex;
  flex-direction: column;
}

.bottom-padding-spacer { height: 40px; }

.small-title {
  font-size: 13.5px;
  font-weight: 600;
  color: var(--md-sys-color-primary);
  margin: 14px 0 4px;
  padding-left: 12px;
  text-transform: uppercase;
}

.arrow-pref-row,
.switch-pref-row,
.dropdown-pref-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  width: 100%;
  gap: 14px;
  padding: 14px 16px;
  box-sizing: border-box;
}

.arrow-pref-row,
.switch-pref-row {
  cursor: pointer;
  transition: background-color 0.2s;
}

.arrow-pref-row:hover,
.switch-pref-row:hover {
  background-color: var(--md-sys-color-surface-container-high);
}

.pref-text {
  display: flex;
  flex-direction: column;
  gap: 2px;
  flex-grow: 1;
  min-width: 0;
}

.pref-title {
  font-size: 15.5px;
  font-weight: 500;
  color: var(--md-sys-color-on-surface);
}

.pref-summary {
  font-size: 12.5px;
  color: var(--md-sys-color-on-surface-variant);
}

.text-ellipsis {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.pref-dropdown {
  max-width: 45%;
  background-color: var(--md-sys-color-surface-container-high);
  color: var(--md-sys-color-on-surface);
  border: 1px solid var(--md-sys-color-outline-variant);
  padding: 6px 12px;
  border-radius: var(--radius-sm);
  outline: none;
  font-size: 13.5px;
  font-weight: 500;
  cursor: pointer;
}

.pref-inner-divider { display: none; }

.apply-card {
  margin-top: 14px;
  padding: 16px;
  flex-direction: row;
  align-items: center;
  gap: 16px;
}

.apply-card md-filled-button {
  flex-shrink: 0;
}
</style>
