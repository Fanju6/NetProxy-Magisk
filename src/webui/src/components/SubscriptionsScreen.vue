<script setup lang="ts">
import { computed, onActivated, onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { moduleClient, type CatalogGroup } from '../api/moduleClient';
import { showToast } from '../utils/ksu';

const { t } = useI18n();
const router = useRouter();
const groups = ref<CatalogGroup[]>([]);
const loading = ref(true);
const operating = ref(false);
const addDialog = ref(false);
const linkDialog = ref(false);
const nodeLink = ref('');
const fileInput = ref<HTMLInputElement | null>(null);

const subscriptions = computed(() => groups.value.filter(group => group.type === 'subscription'));
const localGroups = computed(() => groups.value.filter(group => group.type === 'local'));

const load = async () => {
  try {
    groups.value = await moduleClient.listCatalog();
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

const updateAll = () => run(() => moduleClient.updateAllSubscriptions(), t('subscriptions.updatedAll'));
const updateOne = (id: string) => run(() => moduleClient.updateSubscription(id), t('subscriptions.updated'));
const activate = (id: string) => run(() => moduleClient.activateSubscription(id), t('subscriptions.activated'));

const addNode = async () => {
  const value = nodeLink.value.trim();
  if (!value) return;
  nodeLink.value = '';
  linkDialog.value = false;
  await run(() => moduleClient.addNode(value), t('nodes.importSuccess'));
};

const handleFile = async (event: Event) => {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  input.value = '';
  if (!file) return;
  await run(
    () => file.text().then(content => moduleClient.importContent(content, file.name.replace(/\.[^.]+$/, ''))),
    t('nodes.importSuccess')
  );
};

const usage = (group: CatalogGroup) => {
  const total = group.usage?.total ?? 0;
  const used = (group.usage?.upload ?? 0) + (group.usage?.download ?? 0);
  return { total, used, remaining: Math.max(0, total - used), ratio: total > 0 ? Math.min(1, used / total) : 0 };
};

const formatBytes = (value: number): string => {
  if (value <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const index = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1);
  return `${(value / 1024 ** index).toFixed(index > 1 ? 1 : 0)} ${units[index]}`;
};

const formatTime = (value: string): string => {
  if (!value) return t('common.notSet');
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
};

const statusText = (group: CatalogGroup): string => {
  if (group.progress) return t(`subscriptions.stages.${group.progress.stage}`);
  if (group.last_error) return t('subscriptions.failed');
  if (!group.last_success_at) return t('subscriptions.neverUpdated');
  const expire = group.usage?.expire ?? 0;
  if (expire > 0 && expire <= Date.now() / 1000) return t('subscriptions.expired');
  return t('subscriptions.normal');
};

const updateLink = (event: Event) => {
  nodeLink.value = (event.target as HTMLInputElement).value;
};

onMounted(load);
onActivated(load);
</script>

<template>
  <div class="subscriptions-page">
    <div class="page-actions">
      <md-outlined-button :disabled="operating || subscriptions.length === 0" @click="updateAll">
        {{ t('subscriptions.updateAll') }}
      </md-outlined-button>
      <md-filled-button :disabled="operating" @click="addDialog = true">{{ t('subscriptions.add') }}</md-filled-button>
    </div>

    <div v-if="loading" class="center-state"><md-circular-progress indeterminate></md-circular-progress></div>
    <div v-else-if="groups.length === 0" class="center-state">
      <strong>{{ t('subscriptions.empty') }}</strong>
      <span>{{ t('subscriptions.emptyDesc') }}</span>
    </div>
    <template v-else>
      <section v-if="subscriptions.length" class="group-section">
        <h2>{{ t('subscriptions.urlSubscriptions') }}</h2>
        <article
          v-for="group in subscriptions"
          :key="group.id"
          class="subscription-card"
          @click="router.push(`/subscriptions/${group.id}`)"
        >
          <div class="card-title">
            <div>
              <strong>{{ group.name }}</strong>
              <small>{{ group.node_count }} {{ t('subscriptions.nodesUnit') }} · {{ statusText(group) }}</small>
            </div>
            <span v-if="group.active" class="active-chip">{{ t('subscriptions.active') }}</span>
          </div>
          <template v-if="group.progress">
            <div class="progress-copy">{{ group.progress.message || statusText(group) }}</div>
            <md-linear-progress indeterminate></md-linear-progress>
          </template>
          <template v-else-if="usage(group).total > 0">
            <div class="usage-copy">
              <span>{{ t('subscriptions.used') }} {{ formatBytes(usage(group).used) }}</span>
              <span>{{ t('subscriptions.remaining') }} {{ formatBytes(usage(group).remaining) }}</span>
            </div>
            <md-linear-progress :value="usage(group).ratio"></md-linear-progress>
          </template>
          <div class="card-footer">
            <span>{{ t('subscriptions.nextUpdate') }} {{ formatTime(group.next_update_at) }}</span>
            <div class="card-buttons" @click.stop>
              <md-text-button v-if="!group.active" :disabled="operating || group.node_count === 0" @click="activate(group.id)">{{ t('subscriptions.activate') }}</md-text-button>
              <md-icon-button :disabled="operating" @click="updateOne(group.id)">
                <md-icon><svg viewBox="0 0 24 24"><path d="M17.65 6.35A8 8 0 1 0 20 12h-2a6 6 0 1 1-1.76-4.24L13 11h8V3z" /></svg></md-icon>
              </md-icon-button>
            </div>
          </div>
          <div v-if="group.last_error" class="error-copy">{{ group.last_error }}</div>
        </article>
      </section>

      <section v-if="localGroups.length" class="group-section">
        <h2>{{ t('subscriptions.localGroups') }}</h2>
        <article
          v-for="group in localGroups"
          :key="group.id"
          class="local-row"
          @click="router.push(`/subscriptions/${group.id}`)"
        >
          <span class="local-icon">L</span>
          <span class="local-copy"><strong>{{ group.id === 'default' ? t('subscriptions.localConfig') : group.name }}</strong><small>{{ group.node_count }} {{ t('subscriptions.nodesUnit') }} · {{ formatTime(group.updated_at) }}</small></span>
          <span v-if="group.active" class="active-chip">{{ t('subscriptions.active') }}</span>
          <span class="chevron">›</span>
        </article>
      </section>
    </template>

    <input ref="fileInput" type="file" hidden @change="handleFile" />

    <md-dialog :open="addDialog" @close="addDialog = false">
      <div slot="headline">{{ t('subscriptions.add') }}</div>
      <div slot="content" class="add-list">
        <button type="button" @click="addDialog = false; router.push('/subscriptions/new')"><strong>{{ t('subscriptions.urlSubscription') }}</strong><small>{{ t('subscriptions.urlSubscriptionDesc') }}</small></button>
        <button type="button" @click="addDialog = false; linkDialog = true"><strong>{{ t('subscriptions.singleNode') }}</strong><small>{{ t('subscriptions.singleNodeDesc') }}</small></button>
        <button type="button" @click="addDialog = false; fileInput?.click()"><strong>{{ t('subscriptions.localFile') }}</strong><small>{{ t('subscriptions.localFileDesc') }}</small></button>
      </div>
      <div slot="actions"><md-text-button @click="addDialog = false">{{ t('common.cancel') }}</md-text-button></div>
    </md-dialog>

    <md-dialog :open="linkDialog" @close="linkDialog = false">
      <div slot="headline">{{ t('subscriptions.singleNode') }}</div>
      <div slot="content" class="link-content">
        <md-outlined-text-field :value="nodeLink" type="textarea" rows="4" :label="t('nodes.importLink')" @input="updateLink"></md-outlined-text-field>
      </div>
      <div slot="actions">
        <md-text-button @click="linkDialog = false">{{ t('common.cancel') }}</md-text-button>
        <md-filled-button :disabled="!nodeLink.trim()" @click="addNode">{{ t('nodes.add') }}</md-filled-button>
      </div>
    </md-dialog>
  </div>
</template>

<style scoped>
.subscriptions-page { padding: 4px 14px 96px; }
.page-actions { display: flex; justify-content: flex-end; gap: 10px; padding: 6px 0 16px; }
.group-section { margin-bottom: 22px; }
.group-section h2 { margin: 0 4px 9px; font-size: 13px; font-weight: 600; color: var(--md-sys-color-primary); }
.subscription-card { margin-bottom: 12px; padding: 16px; border: 1px solid var(--md-sys-color-outline-variant); border-radius: 8px; background: var(--md-sys-color-surface); }
.card-title, .card-footer, .usage-copy { display: flex; align-items: center; gap: 12px; }
.card-title > div { display: flex; flex: 1; min-width: 0; flex-direction: column; gap: 4px; }
.card-title strong { font-size: 18px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.card-title small, .local-copy small, .card-footer > span, .usage-copy { color: var(--md-sys-color-on-surface-variant); font-size: 12px; }
.active-chip { flex: 0 0 auto; padding: 4px 8px; border-radius: 8px; font-size: 11px; color: var(--md-sys-color-on-primary-container); background: var(--md-sys-color-primary-container); }
.usage-copy { justify-content: space-between; margin-top: 14px; margin-bottom: 7px; }
.progress-copy { margin: 14px 0 7px; color: var(--md-sys-color-primary); font-size: 12px; }
.card-footer { min-height: 40px; margin-top: 10px; }
.card-footer > span { flex: 1; }
.card-buttons { display: flex; align-items: center; }
.card-buttons svg { width: 22px; fill: currentColor; }
.error-copy { padding-top: 8px; color: var(--md-sys-color-error); font-size: 12px; }
.local-row { min-height: 68px; padding: 8px 4px; display: flex; align-items: center; gap: 12px; border-bottom: 1px solid var(--md-sys-color-outline-variant); }
.local-icon { width: 38px; height: 38px; border-radius: 8px; display: grid; place-items: center; font-weight: 700; color: var(--md-sys-color-on-secondary-container); background: var(--md-sys-color-secondary-container); }
.local-copy { display: flex; flex: 1; min-width: 0; flex-direction: column; gap: 3px; }
.local-copy strong, .local-copy small { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.chevron { font-size: 25px; color: var(--md-sys-color-outline); }
.center-state { min-height: 58vh; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 8px; text-align: center; color: var(--md-sys-color-on-surface-variant); }
.add-list { min-width: min(360px, 78vw); display: flex; flex-direction: column; }
.add-list button { border: 0; padding: 14px 4px; display: flex; flex-direction: column; gap: 4px; text-align: left; color: inherit; background: transparent; }
.add-list small { color: var(--md-sys-color-on-surface-variant); }
.link-content { min-width: min(360px, 78vw); }
</style>
