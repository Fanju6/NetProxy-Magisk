<script setup lang="ts">
import { computed, onActivated, onDeactivated, onMounted, onUnmounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { moduleClient, type ServiceStatus, type TrafficSnapshot } from '../api/moduleClient';
import { showToast } from '../utils/ksu';

const emit = defineEmits<{ navigate: [tab: 'nodes'] }>();
const { t } = useI18n();

const status = ref<ServiceStatus | null>(null);
const traffic = ref<TrafficSnapshot>({ upload: 0, download: 0 });
const uploadSpeed = ref(0);
const downloadSpeed = ref(0);
const loading = ref(true);
const operating = ref(false);
const modeDialog = ref(false);
let timer: number | null = null;
let previousTraffic: TrafficSnapshot | null = null;
let previousAt = 0;

const isRunning = computed(() => status.value?.state === 'ready');
const transitional = computed(() => ['preparing', 'starting', 'stopping'].includes(status.value?.state ?? ''));
const statusText = computed(() => {
  const key = status.value?.state ?? 'stopped';
  return t(`dashboard.states.${key}`);
});
const nodeText = computed(() => {
  if (!status.value || status.value.active_group_node_count <= 0) return t('dashboard.nodeUnselected');
  return status.value.runtime_selected || status.value.selected_node_ref || `Auto/${status.value.active_group_id}`;
});
const uptimeText = computed(() => formatDuration(status.value?.uptime_seconds ?? 0));

const modes = computed(() => [
  { value: 'rule' as const, title: t('dashboard.modeRule'), description: t('dashboard.modeRuleDesc') },
  { value: 'global' as const, title: t('dashboard.modeGlobal'), description: t('dashboard.modeGlobalDesc') },
  { value: 'direct' as const, title: t('dashboard.modeDirect'), description: t('dashboard.modeDirectDesc') }
]);

const formatBytes = (value: number): string => {
  if (!Number.isFinite(value) || value <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const index = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1);
  return `${(value / 1024 ** index).toFixed(index > 1 ? 1 : 0)} ${units[index]}`;
};

const formatDuration = (seconds: number): string => {
  if (seconds <= 0) return '--';
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  if (days > 0) return `${days}d ${hours}h`;
  if (hours > 0) return `${hours}h ${minutes}m`;
  return `${minutes}m ${seconds % 60}s`;
};

const refresh = async () => {
  try {
    const next = await moduleClient.serviceStatus();
    status.value = next;
    if (next.state === 'ready') {
      const snapshot = await moduleClient.trafficSnapshot();
      const now = Date.now();
      if (previousTraffic && previousAt > 0) {
        const seconds = Math.max((now - previousAt) / 1000, 0.2);
        uploadSpeed.value = Math.max(0, (snapshot.upload - previousTraffic.upload) / seconds);
        downloadSpeed.value = Math.max(0, (snapshot.download - previousTraffic.download) / seconds);
      }
      previousTraffic = snapshot;
      previousAt = now;
      traffic.value = snapshot;
    } else {
      previousTraffic = null;
      previousAt = 0;
      uploadSpeed.value = 0;
      downloadSpeed.value = 0;
    }
  } catch (error) {
    console.warn('Failed to refresh dashboard:', error);
  } finally {
    loading.value = false;
  }
};

const toggleService = async () => {
  if (operating.value || transitional.value) return;
  operating.value = true;
  try {
    await moduleClient.serviceAction(isRunning.value ? 'stop' : 'start');
    await refresh();
  } catch (error) {
    showToast(error instanceof Error ? error.message : String(error));
  } finally {
    operating.value = false;
  }
};

const selectMode = async (mode: 'rule' | 'global' | 'direct') => {
  modeDialog.value = false;
  try {
    await moduleClient.setMode(mode);
    await refresh();
    showToast(t('dashboard.modeSwitched', { mode: mode.toUpperCase() }));
  } catch (error) {
    showToast(error instanceof Error ? error.message : String(error));
  }
};

const startPolling = () => {
  if (timer !== null) return;
  void refresh();
  timer = window.setInterval(refresh, 1500);
};

const stopPolling = () => {
  if (timer !== null) window.clearInterval(timer);
  timer = null;
};

onMounted(startPolling);
onActivated(startPolling);
onDeactivated(stopPolling);
onUnmounted(stopPolling);
</script>

<template>
  <div class="dashboard-page">
    <section class="service-panel" :class="`state-${status?.state ?? 'stopped'}`">
      <div class="service-copy">
        <span class="eyebrow">{{ t('dashboard.serviceStatus') }}</span>
        <div class="service-line">
          <span class="status-dot"></span>
          <strong>{{ loading ? t('dashboard.loading') : statusText }}</strong>
        </div>
        <span class="uptime">{{ t('dashboard.uptimePrefix') }} {{ uptimeText }}</span>
      </div>
      <button class="power-button" :disabled="operating || transitional" type="button" @click="toggleService">
        <svg viewBox="0 0 24 24"><path d="M13 3h-2v10h2V3zm4.83 2.17-1.42 1.42A7 7 0 1 1 7.58 6.58L6.17 5.17A9 9 0 1 0 17.83 5.17z" /></svg>
      </button>
    </section>

    <section class="traffic-band">
      <div class="traffic-item">
        <span>{{ t('dashboard.download') }}</span>
        <strong>{{ formatBytes(downloadSpeed) }}/s</strong>
        <small>{{ formatBytes(traffic.download) }}</small>
      </div>
      <div class="traffic-divider"></div>
      <div class="traffic-item">
        <span>{{ t('dashboard.upload') }}</span>
        <strong>{{ formatBytes(uploadSpeed) }}/s</strong>
        <small>{{ formatBytes(traffic.upload) }}</small>
      </div>
    </section>

    <section class="detail-list">
      <button class="detail-row" type="button" @click="emit('navigate', 'nodes')">
        <span class="detail-icon">N</span>
        <span class="detail-copy">
          <small>{{ t('dashboard.currentNode') }}</small>
          <strong>{{ nodeText }}</strong>
        </span>
        <span class="chevron">›</span>
      </button>
      <button class="detail-row" type="button" @click="modeDialog = true">
        <span class="detail-icon">M</span>
        <span class="detail-copy">
          <small>{{ t('dashboard.outboundMode') }}</small>
          <strong>{{ (status?.outbound_mode ?? 'rule').toUpperCase() }}</strong>
        </span>
        <span class="chevron">›</span>
      </button>
      <div v-if="status?.error" class="service-error">{{ status.error }}</div>
    </section>

    <md-dialog :open="modeDialog" @close="modeDialog = false">
      <div slot="headline">{{ t('dashboard.outboundMode') }}</div>
      <div slot="content" class="mode-list">
        <button v-for="mode in modes" :key="mode.value" type="button" @click="selectMode(mode.value)">
          <span><strong>{{ mode.title }}</strong><small>{{ mode.description }}</small></span>
          <span v-if="status?.outbound_mode === mode.value" class="selected-mark">✓</span>
        </button>
      </div>
      <div slot="actions"><md-text-button @click="modeDialog = false">{{ t('common.cancel') }}</md-text-button></div>
    </md-dialog>
  </div>
</template>

<style scoped>
.dashboard-page { padding: 12px 16px 96px; display: flex; flex-direction: column; gap: 14px; }
.service-panel { min-height: 132px; padding: 22px; border-radius: 8px; display: flex; align-items: center; justify-content: space-between; color: var(--md-sys-color-on-primary-container); background: var(--md-sys-color-primary-container); }
.service-copy { display: flex; flex-direction: column; gap: 6px; min-width: 0; }
.eyebrow, .uptime { font-size: 13px; opacity: .74; }
.service-line { display: flex; align-items: center; gap: 10px; font-size: 25px; }
.status-dot { width: 10px; height: 10px; border-radius: 50%; background: #2e9d64; box-shadow: 0 0 0 5px color-mix(in srgb, #2e9d64 18%, transparent); }
.state-stopped .status-dot, .state-failed .status-dot { background: var(--md-sys-color-error); box-shadow: 0 0 0 5px color-mix(in srgb, var(--md-sys-color-error) 18%, transparent); }
.state-starting .status-dot, .state-preparing .status-dot, .state-stopping .status-dot { background: #e59b20; animation: pulse 1.2s infinite; }
.power-button { width: 54px; height: 54px; flex: 0 0 auto; border: 0; border-radius: 50%; display: grid; place-items: center; color: var(--md-sys-color-on-primary); background: var(--md-sys-color-primary); }
.power-button:disabled { opacity: .48; }
.power-button svg { width: 26px; height: 26px; fill: currentColor; }
.traffic-band { display: grid; grid-template-columns: 1fr 1px 1fr; align-items: stretch; padding: 18px 8px; }
.traffic-item { display: flex; flex-direction: column; align-items: center; gap: 4px; }
.traffic-item span, .traffic-item small { color: var(--md-sys-color-on-surface-variant); font-size: 12px; }
.traffic-item strong { font-size: 20px; }
.traffic-divider { background: var(--md-sys-color-outline-variant); }
.detail-list { border-top: 1px solid var(--md-sys-color-outline-variant); }
.detail-row { width: 100%; min-height: 70px; border: 0; border-bottom: 1px solid var(--md-sys-color-outline-variant); padding: 12px 4px; display: flex; align-items: center; gap: 14px; text-align: left; color: inherit; background: transparent; }
.detail-icon { width: 36px; height: 36px; border-radius: 8px; display: grid; place-items: center; font-weight: 700; color: var(--md-sys-color-on-secondary-container); background: var(--md-sys-color-secondary-container); }
.detail-copy { display: flex; flex: 1; min-width: 0; flex-direction: column; gap: 3px; }
.detail-copy small { color: var(--md-sys-color-on-surface-variant); }
.detail-copy strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.chevron { font-size: 26px; color: var(--md-sys-color-outline); }
.service-error { margin-top: 12px; padding: 12px; border-radius: 8px; color: var(--md-sys-color-on-error-container); background: var(--md-sys-color-error-container); }
.mode-list { display: flex; flex-direction: column; min-width: min(340px, 76vw); }
.mode-list button { border: 0; padding: 14px 4px; display: flex; align-items: center; gap: 16px; text-align: left; color: inherit; background: transparent; }
.mode-list button span:first-child { display: flex; flex: 1; flex-direction: column; gap: 3px; }
.mode-list small { color: var(--md-sys-color-on-surface-variant); }
.selected-mark { color: var(--md-sys-color-primary); font-size: 20px; }
@keyframes pulse { 50% { opacity: .4; } }
</style>
