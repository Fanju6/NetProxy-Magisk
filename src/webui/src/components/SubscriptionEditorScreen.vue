<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import {
  moduleClient,
  type SubscriptionDraft,
  type SubscriptionEditor
} from '../api/moduleClient';
import { showToast } from '../utils/ksu';

const props = defineProps<{ id: string }>();
const router = useRouter();
const { t } = useI18n();
const isNew = computed(() => props.id === 'new' || props.id === '');
const loading = ref(!isNew.value);
const saving = ref(false);
const advanced = ref(false);
const original = ref<SubscriptionEditor | undefined>();
const headersText = ref('');
const draft = ref<Required<SubscriptionDraft>>({
  name: '',
  url: '',
  userAgent: '',
  hwid: '',
  customHeaders: {},
  autoUpdate: true,
  updateInterval: 86400,
  updateViaProxy: 'auto',
  include: '',
  exclude: '',
  allowInsecure: false,
  timeout: 60
});

const load = async () => {
  if (isNew.value) return;
  try {
    const value = await moduleClient.editSubscriptionData(props.id);
    original.value = value;
    draft.value = {
      name: value.name,
      url: value.url,
      userAgent: value.user_agent,
      hwid: value.hwid,
      customHeaders: value.custom_headers,
      autoUpdate: value.auto_update,
      updateInterval: value.update_interval,
      updateViaProxy: value.update_via_proxy,
      include: value.include,
      exclude: value.exclude,
      allowInsecure: value.allow_insecure,
      timeout: value.timeout
    };
    headersText.value = Object.entries(value.custom_headers)
      .map(([key, val]) => `${key}: ${val}`)
      .join('\n');
  } catch (error) {
    showToast(error instanceof Error ? error.message : String(error));
  } finally {
    loading.value = false;
  }
};

const parseHeaders = (): Record<string, string> => {
  const result: Record<string, string> = {};
  headersText.value.split(/\r?\n/).forEach((raw, index) => {
    const line = raw.trim();
    if (!line) return;
    const separator = line.indexOf(':');
    if (separator <= 0) throw new Error(t('subscriptions.headerFormat', { line: index + 1 }));
    const key = line.slice(0, separator).trim();
    const value = line.slice(separator + 1).trim();
    if (!key || !value) throw new Error(t('subscriptions.headerFormat', { line: index + 1 }));
    result[key] = value;
  });
  return result;
};

const switchSelected = (event: Event): boolean =>
  Boolean((event.target as unknown as { selected: boolean }).selected);

const save = async () => {
  if (saving.value) return;
  const value = { ...draft.value, name: draft.value.name.trim(), url: draft.value.url.trim() };
  if (!value.name || !value.url) {
    showToast(t('nodes.nameUrlRequired'));
    return;
  }
  if (value.updateInterval < 900 || value.timeout <= 0) {
    showToast(t('subscriptions.invalidTiming'));
    return;
  }
  try {
    value.customHeaders = parseHeaders();
  } catch (error) {
    showToast(error instanceof Error ? error.message : String(error));
    return;
  }
  saving.value = true;
  try {
    if (isNew.value) await moduleClient.addSubscription(value);
    else await moduleClient.editSubscription(props.id, value, original.value);
    showToast(t('subscriptions.saved'));
    router.back();
  } catch (error) {
    showToast(error instanceof Error ? error.message : String(error));
  } finally {
    saving.value = false;
  }
};

onMounted(load);
</script>

<template>
  <div class="editor-page">
    <header class="sub-top-bar">
      <md-icon-button @click="router.back()"><md-icon><svg viewBox="0 0 24 24"><path d="M20 11H7.83l5.59-5.59L12 4l-8 8 8 8 1.42-1.41L7.83 13H20z" /></svg></md-icon></md-icon-button>
      <h1>{{ isNew ? t('subscriptions.addUrl') : t('subscriptions.edit') }}</h1>
      <md-icon-button :disabled="saving || loading" @click="save"><md-icon><svg viewBox="0 0 24 24"><path d="M9 16.17 4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41z" /></svg></md-icon></md-icon-button>
    </header>

    <main class="editor-content">
      <div v-if="loading" class="center-state"><md-circular-progress indeterminate></md-circular-progress></div>
      <template v-else>
        <section class="form-section">
          <label><span>{{ t('subscriptions.name') }}</span><input v-model="draft.name" autocomplete="off" /></label>
          <label><span>{{ t('subscriptions.url') }}</span><textarea v-model="draft.url" rows="3" spellcheck="false"></textarea></label>
        </section>

        <section class="form-section">
          <label class="switch-row">
            <span><strong>{{ t('subscriptions.autoUpdate') }}</strong><small>{{ t('subscriptions.autoUpdateDesc') }}</small></span>
            <md-switch icons :selected="draft.autoUpdate" @change="draft.autoUpdate = switchSelected($event)"></md-switch>
          </label>
          <label><span>{{ t('subscriptions.interval') }}</span><input v-model.number="draft.updateInterval" type="number" min="900" step="900" /></label>
          <label><span>{{ t('subscriptions.updateViaProxy') }}</span><select v-model="draft.updateViaProxy"><option value="auto">Auto</option><option value="always">Always</option><option value="never">Never</option></select></label>
          <label><span>{{ t('subscriptions.timeout') }}</span><input v-model.number="draft.timeout" type="number" min="1" /></label>
        </section>

        <button class="advanced-toggle" type="button" @click="advanced = !advanced">
          <span>{{ t('subscriptions.advanced') }}</span><span>{{ advanced ? '−' : '+' }}</span>
        </button>
        <section v-if="advanced" class="form-section">
          <label><span>User-Agent</span><input v-model="draft.userAgent" autocomplete="off" /></label>
          <label><span>HWID</span><input v-model="draft.hwid" autocomplete="off" /></label>
          <label><span>{{ t('subscriptions.headers') }}</span><textarea v-model="headersText" rows="4" :placeholder="t('subscriptions.headersPlaceholder')" spellcheck="false"></textarea></label>
          <label><span>{{ t('subscriptions.include') }}</span><input v-model="draft.include" autocomplete="off" /></label>
          <label><span>{{ t('subscriptions.exclude') }}</span><input v-model="draft.exclude" autocomplete="off" /></label>
          <label class="switch-row">
            <span><strong>{{ t('subscriptions.allowInsecure') }}</strong><small>{{ t('subscriptions.allowInsecureDesc') }}</small></span>
            <md-switch icons :selected="draft.allowInsecure" @change="draft.allowInsecure = switchSelected($event)"></md-switch>
          </label>
        </section>
      </template>
    </main>
  </div>
</template>

<style scoped>
.editor-page { min-height: 100vh; background: var(--md-sys-color-background); }
.sub-top-bar { height: calc(64px + var(--top-inset)); padding: var(--top-inset) 8px 0; box-sizing: border-box; display: flex; align-items: center; position: sticky; top: 0; z-index: 20; background: var(--md-sys-color-background); }
.sub-top-bar h1 { flex: 1; margin: 0 8px; font-size: 21px; }
.sub-top-bar svg { width: 22px; fill: currentColor; }
.editor-content { padding: 12px 16px calc(30px + var(--bottom-inset)); }
.form-section { margin-bottom: 14px; border: 1px solid var(--md-sys-color-outline-variant); border-radius: 8px; overflow: hidden; }
.form-section > label { min-height: 64px; padding: 12px 14px; box-sizing: border-box; display: flex; flex-direction: column; gap: 7px; border-bottom: 1px solid var(--md-sys-color-outline-variant); }
.form-section > label:last-child { border-bottom: 0; }
.form-section label > span:first-child { color: var(--md-sys-color-on-surface-variant); font-size: 13px; }
input, textarea, select { width: 100%; box-sizing: border-box; border: 0; outline: 0; padding: 0; resize: vertical; font: inherit; color: var(--md-sys-color-on-surface); background: transparent; }
textarea { line-height: 1.45; }
.switch-row { flex-direction: row !important; align-items: center; }
.switch-row > span { display: flex; flex: 1; flex-direction: column; gap: 3px; }
.switch-row strong { color: var(--md-sys-color-on-surface); font-size: 15px; }
.switch-row small { color: var(--md-sys-color-on-surface-variant); }
.advanced-toggle { width: 100%; min-height: 54px; margin-bottom: 14px; border: 0; border-bottom: 1px solid var(--md-sys-color-outline-variant); padding: 0 4px; display: flex; align-items: center; justify-content: space-between; color: var(--md-sys-color-primary); background: transparent; }
.center-state { min-height: 60vh; display: grid; place-items: center; }
</style>
