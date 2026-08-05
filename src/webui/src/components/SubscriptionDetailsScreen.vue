<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import {
  moduleClient,
  type CatalogNodeGroup,
  type SubscriptionHistoryEntry
} from '../api/moduleClient';
import { showToast } from '../utils/ksu';

const props = defineProps<{ id: string }>();
const { t } = useI18n();
const router = useRouter();
const details = ref<CatalogNodeGroup | null>(null);
const history = ref<SubscriptionHistoryEntry[]>([]);
const loading = ref(true);
const operating = ref(false);
const deleteDialog = ref(false);

const load = async () => {
  loading.value = true;
  try {
    const next = await moduleClient.showCatalog(props.id);
    details.value = next;
    history.value = next.group.type === 'subscription'
      ? await moduleClient.subscriptionHistory(props.id)
      : [];
  } catch (error) {
    showToast(error instanceof Error ? error.message : String(error));
  } finally {
    loading.value = false;
  }
};

const run = async (action: () => Promise<unknown>, message: string) => {
  if (operating.value) return;
  operating.value = true;
  try {
    await action();
    await load();
    showToast(message);
  } catch (error) {
    showToast(error instanceof Error ? error.message : String(error));
  } finally {
    operating.value = false;
  }
};

const remove = async () => {
  deleteDialog.value = false;
  try {
    await moduleClient.removeSubscription(props.id);
    router.back();
  } catch (error) {
    showToast(error instanceof Error ? error.message : String(error));
  }
};

const formatTime = (value: string): string => {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
};

onMounted(load);
</script>

<template>
  <div class="sub-page">
    <header class="sub-top-bar">
      <md-icon-button @click="router.back()"><md-icon><svg viewBox="0 0 24 24"><path d="M20 11H7.83l5.59-5.59L12 4l-8 8 8 8 1.42-1.41L7.83 13H20z" /></svg></md-icon></md-icon-button>
      <h1>{{ details?.group.name || t('subscriptions.details') }}</h1>
      <div class="top-actions" v-if="details?.group.type === 'subscription'">
        <md-icon-button @click="router.push(`/subscriptions/${props.id}/edit`)"><md-icon><svg viewBox="0 0 24 24"><path d="M3 17.25V21h3.75L17.81 9.94l-3.75-3.75L3 17.25zM20.71 7.04a1 1 0 0 0 0-1.41l-2.34-2.34a1 1 0 0 0-1.41 0l-1.83 1.83 3.75 3.75z" /></svg></md-icon></md-icon-button>
        <md-icon-button :disabled="operating" @click="run(() => moduleClient.updateSubscription(props.id), t('subscriptions.updated'))"><md-icon><svg viewBox="0 0 24 24"><path d="M17.65 6.35A8 8 0 1 0 20 12h-2a6 6 0 1 1-1.76-4.24L13 11h8V3z" /></svg></md-icon></md-icon-button>
      </div>
    </header>

    <main class="sub-content">
      <div v-if="loading" class="center-state"><md-circular-progress indeterminate></md-circular-progress></div>
      <template v-else-if="details">
        <section class="summary-band">
          <div><small>{{ t('subscriptions.type') }}</small><strong>{{ details.group.type === 'subscription' ? t('subscriptions.urlSubscription') : t('subscriptions.localConfig') }}</strong></div>
          <div><small>{{ t('subscriptions.nodeCount') }}</small><strong>{{ details.nodes.length }}</strong></div>
          <div><small>{{ t('subscriptions.state') }}</small><strong>{{ details.group.active ? t('subscriptions.active') : t('subscriptions.inactive') }}</strong></div>
        </section>

        <div v-if="!details.group.active && details.nodes.length" class="activate-row">
          <span>{{ t('subscriptions.activateDesc') }}</span>
          <md-filled-button :disabled="operating" @click="run(() => moduleClient.activateSubscription(props.id), t('subscriptions.activated'))">{{ t('subscriptions.activate') }}</md-filled-button>
        </div>

        <section class="section">
          <h2>{{ t('subscriptions.nodes') }}</h2>
          <div class="node-row" v-for="node in details.nodes" :key="node.tag">
            <span class="node-icon">{{ (node.protocol || '?')[0].toUpperCase() }}</span>
            <span><strong>{{ node.tag }}</strong><small>{{ node.protocol.toUpperCase() }} · {{ node.server }}:{{ node.port }}</small></span>
          </div>
          <p v-if="details.nodes.length === 0" class="empty-copy">{{ t('nodes.emptyGroup') }}</p>
        </section>

        <section v-if="details.group.type === 'subscription'" class="section">
          <h2>{{ t('subscriptions.history') }}</h2>
          <div class="history-row" v-for="entry in [...history].reverse()" :key="`${entry.time}-${entry.revision}`">
            <span class="history-dot" :class="entry.status"></span>
            <span><strong>{{ entry.message }}</strong><small>{{ formatTime(entry.time) }}<template v-if="entry.node_count != null"> · {{ entry.node_count }} {{ t('subscriptions.nodesUnit') }}</template></small></span>
          </div>
          <p v-if="history.length === 0" class="empty-copy">{{ t('subscriptions.noHistory') }}</p>
          <md-text-button class="delete-button" @click="deleteDialog = true">{{ t('subscriptions.remove') }}</md-text-button>
        </section>
      </template>
    </main>

    <md-dialog :open="deleteDialog" @close="deleteDialog = false">
      <div slot="headline">{{ t('subscriptions.removeTitle') }}</div>
      <div slot="content">{{ t('subscriptions.removeDesc') }}</div>
      <div slot="actions">
        <md-text-button @click="deleteDialog = false">{{ t('common.cancel') }}</md-text-button>
        <md-filled-button @click="remove">{{ t('common.delete') }}</md-filled-button>
      </div>
    </md-dialog>
  </div>
</template>

<style scoped>
.sub-page { min-height: 100vh; background: var(--md-sys-color-background); }
.sub-top-bar { height: calc(64px + var(--top-inset)); padding: var(--top-inset) 8px 0; box-sizing: border-box; display: flex; align-items: center; position: sticky; top: 0; z-index: 20; background: var(--md-sys-color-background); }
.sub-top-bar h1 { flex: 1; margin: 0 8px; font-size: 21px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.sub-top-bar svg { width: 22px; fill: currentColor; }
.top-actions { display: flex; }
.sub-content { padding: 12px 16px calc(28px + var(--bottom-inset)); }
.summary-band { display: grid; grid-template-columns: repeat(3, 1fr); gap: 8px; padding: 18px 0; border-bottom: 1px solid var(--md-sys-color-outline-variant); }
.summary-band div { display: flex; min-width: 0; flex-direction: column; align-items: center; gap: 5px; }
.summary-band small { color: var(--md-sys-color-on-surface-variant); }
.summary-band strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 100%; }
.activate-row { margin: 14px 0; padding: 12px 0; display: flex; align-items: center; gap: 12px; }
.activate-row span { flex: 1; color: var(--md-sys-color-on-surface-variant); }
.section { margin-top: 22px; }
.section h2 { margin: 0 4px 8px; font-size: 13px; color: var(--md-sys-color-primary); }
.node-row, .history-row { min-height: 64px; display: flex; align-items: center; gap: 12px; border-bottom: 1px solid var(--md-sys-color-outline-variant); }
.node-icon { width: 36px; height: 36px; border-radius: 8px; display: grid; place-items: center; font-weight: 700; background: var(--md-sys-color-secondary-container); }
.node-row > span:last-child, .history-row > span:last-child { display: flex; min-width: 0; flex: 1; flex-direction: column; gap: 3px; }
.node-row strong, .node-row small { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.node-row small, .history-row small, .empty-copy { color: var(--md-sys-color-on-surface-variant); }
.history-dot { width: 9px; height: 9px; border-radius: 50%; background: var(--md-sys-color-primary); }
.history-dot.failed, .history-dot.error { background: var(--md-sys-color-error); }
.delete-button { width: 100%; margin-top: 14px; }
.center-state { min-height: 60vh; display: grid; place-items: center; }
</style>
