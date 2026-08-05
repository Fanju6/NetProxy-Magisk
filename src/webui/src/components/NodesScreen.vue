<script setup lang="ts">
import { computed, onActivated, onMounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { moduleClient, type CatalogNode, type CatalogNodeGroup, type NodeSelection } from '../api/moduleClient';
import { showToast } from '../utils/ksu';

const { t } = useI18n();
const groups = ref<CatalogNodeGroup[]>([]);
const selection = ref<NodeSelection>({ active_group_id: '', selector_mode: 'urltest', selected: '' });
const selectedGroupId = ref(localStorage.getItem('np_catalog_group') ?? '');
const loading = ref(true);
const operating = ref(false);
const addDialog = ref(false);
const actionDialog = ref(false);
const link = ref('');
const actionNode = ref<{ group: CatalogNodeGroup; node: CatalogNode } | null>(null);
const fileInput = ref<HTMLInputElement | null>(null);

const selectedGroup = computed(() =>
  groups.value.find(item => item.group.id === selectedGroupId.value) ?? groups.value[0] ?? null
);

const isNodeSelected = (groupId: string, tag: string) =>
  selection.value.selector_mode === 'manual' && selection.value.selected === `${groupId}/${tag}`;

const load = async () => {
  try {
    const [nextGroups, nextSelection] = await Promise.all([
      moduleClient.listNodes(),
      moduleClient.currentNode()
    ]);
    groups.value = nextGroups;
    selection.value = nextSelection;
    if (!nextGroups.some(item => item.group.id === selectedGroupId.value)) {
      selectedGroupId.value = nextSelection.active_group_id || nextGroups[0]?.group.id || '';
    }
  } catch (error) {
    showToast(error instanceof Error ? error.message : String(error));
  } finally {
    loading.value = false;
  }
};

const chooseGroup = (id: string) => {
  selectedGroupId.value = id;
  localStorage.setItem('np_catalog_group', id);
};

const run = async (action: () => Promise<unknown>, success: string) => {
  if (operating.value) return;
  operating.value = true;
  try {
    await action();
    await load();
    showToast(success);
  } catch (error) {
    showToast(error instanceof Error ? error.message : String(error));
  } finally {
    operating.value = false;
  }
};

const useAuto = (groupId: string) => run(
  () => moduleClient.selectAuto(groupId),
  t('nodes.autoSelected')
);

const useNode = (groupId: string, tag: string) => run(
  () => moduleClient.selectNode(groupId, tag),
  t('nodes.switchedTo', { tag })
);

const addLink = async () => {
  const value = link.value.trim();
  if (!value) return;
  addDialog.value = false;
  link.value = '';
  await run(() => moduleClient.addNode(value), t('nodes.importSuccess'));
};

const handleFile = async (event: Event) => {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  input.value = '';
  if (!file) return;
  const content = await file.text();
  const groupName = file.name.replace(/\.[^.]+$/, '');
  await run(() => moduleClient.importContent(content, groupName), t('nodes.importSuccess'));
};

const openNodeActions = (group: CatalogNodeGroup, node: CatalogNode) => {
  actionNode.value = { group, node };
  actionDialog.value = true;
};

const copyNode = async () => {
  const current = actionNode.value;
  if (!current) return;
  actionDialog.value = false;
  await run(
    () => moduleClient.copyNode(`${current.group.group.id}/${current.node.tag}`),
    t('nodes.copiedToLocal')
  );
};

const removeNode = async () => {
  const current = actionNode.value;
  if (!current || current.group.group.type !== 'local') return;
  actionDialog.value = false;
  await run(
    () => moduleClient.removeNode(`${current.group.group.id}/${current.node.tag}`),
    t('nodes.nodeDeleted')
  );
};

const delay = () => run(
  () => moduleClient.testDelay(),
  t('nodes.testComplete')
);

onMounted(load);
onActivated(load);
</script>

<template>
  <div class="nodes-page">
    <div class="nodes-toolbar">
      <div class="group-tabs" role="tablist">
        <button
          v-for="item in groups"
          :key="item.group.id"
          type="button"
          :class="{ active: item.group.id === selectedGroup?.group.id }"
          @click="chooseGroup(item.group.id)"
        >
          {{ item.group.id === 'default' ? t('subscriptions.localConfig') : item.group.name }}
          <small>{{ item.nodes.length }}</small>
        </button>
      </div>
      <div class="toolbar-actions">
        <md-icon-button :disabled="operating" @click="delay">
          <md-icon><svg viewBox="0 0 24 24"><path d="M15 1H9v2h6V1zm-1 13h-4v2h4v-2zm3.03-7.03 1.42-1.42a10.02 10.02 0 0 0-1.41-1.41l-1.42 1.42A8.94 8.94 0 0 0 12 4a9 9 0 1 0 9 9 8.94 8.94 0 0 0-1.56-5.06l-1.42 1.42A6.94 6.94 0 0 1 19 13a7 7 0 1 1-7-7 6.94 6.94 0 0 1 3.64 1.02z" /></svg></md-icon>
        </md-icon-button>
        <md-icon-button :disabled="operating" @click="addDialog = true">
          <md-icon><svg viewBox="0 0 24 24"><path d="M19 13h-6v6h-2v-6H5v-2h6V5h2v6h6v2z" /></svg></md-icon>
        </md-icon-button>
      </div>
    </div>

    <div v-if="loading" class="center-state"><md-circular-progress indeterminate></md-circular-progress></div>
    <div v-else-if="!selectedGroup" class="center-state">
      <strong>{{ t('nodes.emptyGroups') }}</strong>
      <span>{{ t('nodes.emptyGroupsDesc') }}</span>
    </div>
    <template v-else>
      <button
        class="auto-row"
        :class="{ selected: selection.selector_mode === 'urltest' && selection.active_group_id === selectedGroup.group.id }"
        type="button"
        :disabled="operating || selectedGroup.nodes.length === 0"
        @click="useAuto(selectedGroup.group.id)"
      >
        <span class="node-avatar auto">A</span>
        <span class="node-copy"><strong>Auto-Fastest</strong><small>{{ t('nodes.autoDescription') }}</small></span>
        <span class="select-mark">✓</span>
      </button>

      <div v-if="selectedGroup.nodes.length === 0" class="center-state compact">
        <strong>{{ t('nodes.emptyGroup') }}</strong>
      </div>
      <div v-else class="node-list">
        <button
          v-for="node in selectedGroup.nodes"
          :key="`${selectedGroup.group.id}/${node.tag}`"
          class="node-row"
          :class="{ selected: isNodeSelected(selectedGroup.group.id, node.tag) }"
          type="button"
          :disabled="operating"
          @click="useNode(selectedGroup.group.id, node.tag)"
          @contextmenu.prevent="openNodeActions(selectedGroup, node)"
        >
          <span class="node-avatar">{{ (node.protocol || '?').slice(0, 1).toUpperCase() }}</span>
          <span class="node-copy"><strong>{{ node.tag }}</strong><small>{{ node.protocol.toUpperCase() }} · {{ node.server }}:{{ node.port }}</small></span>
          <span class="select-mark">✓</span>
          <span class="more" @click.stop="openNodeActions(selectedGroup, node)">⋮</span>
        </button>
      </div>
    </template>

    <input ref="fileInput" type="file" hidden @change="handleFile" />

    <md-dialog :open="addDialog" @close="addDialog = false">
      <div slot="headline">{{ t('nodes.menuAddNode') }}</div>
      <div slot="content" class="add-content">
        <md-outlined-text-field
          :value="link"
          type="textarea"
          rows="4"
          :label="t('nodes.importLink')"
          @input="link = ($event.target as HTMLInputElement).value"
        ></md-outlined-text-field>
        <md-outlined-button @click="fileInput?.click()">{{ t('nodes.importFile') }}</md-outlined-button>
      </div>
      <div slot="actions">
        <md-text-button @click="addDialog = false">{{ t('common.cancel') }}</md-text-button>
        <md-filled-button :disabled="!link.trim()" @click="addLink">{{ t('nodes.add') }}</md-filled-button>
      </div>
    </md-dialog>

    <md-dialog :open="actionDialog" @close="actionDialog = false">
      <div slot="headline">{{ actionNode?.node.tag }}</div>
      <div slot="content" class="action-list">
        <button v-if="actionNode?.group.group.type === 'subscription'" type="button" @click="copyNode">{{ t('nodes.copyToLocal') }}</button>
        <button v-else class="danger" type="button" @click="removeNode">{{ t('common.delete') }}</button>
      </div>
      <div slot="actions"><md-text-button @click="actionDialog = false">{{ t('common.cancel') }}</md-text-button></div>
    </md-dialog>
  </div>
</template>

<style scoped>
.nodes-page { padding: 0 14px 96px; }
.nodes-toolbar { position: sticky; top: 0; z-index: 8; display: flex; align-items: center; gap: 8px; padding: 8px 0 12px; background: var(--md-sys-color-background); }
.group-tabs { display: flex; flex: 1; min-width: 0; gap: 8px; overflow-x: auto; scrollbar-width: none; }
.group-tabs button { min-height: 38px; padding: 0 13px; flex: 0 0 auto; border: 1px solid var(--md-sys-color-outline-variant); border-radius: 8px; color: var(--md-sys-color-on-surface-variant); background: transparent; }
.group-tabs button.active { color: var(--md-sys-color-on-secondary-container); border-color: transparent; background: var(--md-sys-color-secondary-container); }
.group-tabs small { margin-left: 5px; opacity: .68; }
.toolbar-actions { display: flex; }
.toolbar-actions svg { width: 22px; fill: currentColor; }
.auto-row, .node-row { width: 100%; min-height: 72px; border: 0; border-bottom: 1px solid var(--md-sys-color-outline-variant); padding: 10px 4px; display: flex; align-items: center; gap: 12px; text-align: left; color: inherit; background: transparent; }
.auto-row { margin-top: 2px; }
.node-avatar { width: 40px; height: 40px; flex: 0 0 auto; border-radius: 8px; display: grid; place-items: center; font-weight: 700; color: var(--md-sys-color-on-tertiary-container); background: var(--md-sys-color-tertiary-container); }
.node-avatar.auto { color: var(--md-sys-color-on-primary-container); background: var(--md-sys-color-primary-container); }
.node-copy { display: flex; flex: 1; min-width: 0; flex-direction: column; gap: 4px; }
.node-copy strong, .node-copy small { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.node-copy small { color: var(--md-sys-color-on-surface-variant); }
.select-mark { visibility: hidden; color: var(--md-sys-color-primary); font-size: 20px; }
.selected .select-mark { visibility: visible; }
.more { width: 28px; text-align: center; font-size: 24px; color: var(--md-sys-color-on-surface-variant); }
.center-state { min-height: 55vh; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 8px; color: var(--md-sys-color-on-surface-variant); text-align: center; }
.center-state.compact { min-height: 180px; }
.add-content { display: flex; min-width: min(360px, 78vw); flex-direction: column; gap: 14px; }
.action-list { min-width: min(320px, 72vw); }
.action-list button { width: 100%; border: 0; padding: 14px 4px; text-align: left; color: inherit; background: transparent; }
.action-list .danger { color: var(--md-sys-color-error); }
</style>
