<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { setEdgeToEdge } from './utils/ksu';
import '@material/web/icon/icon.js';

type TabId = 'dashboard' | 'nodes' | 'subscriptions' | 'settings';

const route = useRoute();
const router = useRouter();
const { t } = useI18n();
const keyboardOpen = ref(false);
let viewportBaseline = 0;

const activeTab = computed<TabId>(() => {
  if (route.path.startsWith('/nodes')) return 'nodes';
  if (route.path.startsWith('/subscriptions')) return 'subscriptions';
  if (route.path.startsWith('/settings')) return 'settings';
  return 'dashboard';
});
const childPage = computed(() => route.meta.showBack === true);
const hideBottomNav = computed(() => childPage.value || keyboardOpen.value);
const mainHeight = computed(() => {
  if (childPage.value) return '100vh';
  if (keyboardOpen.value) return 'calc(100vh - 64px - var(--top-inset))';
  return 'calc(100vh - 64px - var(--top-inset) - var(--bottom-nav-height) - var(--bottom-inset))';
});
const screenTitle = computed(() => t(`nav.${activeTab.value}`));

const routes: Record<TabId, string> = {
  dashboard: '/dashboard',
  nodes: '/nodes',
  subscriptions: '/subscriptions',
  settings: '/settings'
};
const tabOrder: TabId[] = ['dashboard', 'nodes', 'subscriptions', 'settings'];

const icons: Record<TabId, { regular: string; filled: string }> = {
  dashboard: {
    regular: 'M12 5.69l5 3.64V18h-2v-6H9v6H7V9.33l5-3.64M12 3 2 12h3v8h6v-6h2v6h6v-8h3L12 3z',
    filled: 'M10 20v-6h4v6h5v-8h3L12 3 2 12h3v8z'
  },
  nodes: {
    regular: 'M20 13H4a1 1 0 0 0-1 1v6a1 1 0 0 0 1 1h16a1 1 0 0 0 1-1v-6a1 1 0 0 0-1-1zm-1 6H5v-4h14v4zM20 3H4a1 1 0 0 0-1 1v6a1 1 0 0 0 1 1h16a1 1 0 0 0 1-1V4a1 1 0 0 0-1-1zm-1 6H5V5h14v4z',
    filled: 'M20 13H4a1 1 0 0 0-1 1v6a1 1 0 0 0 1 1h16a1 1 0 0 0 1-1v-6a1 1 0 0 0-1-1zM20 3H4a1 1 0 0 0-1 1v6a1 1 0 0 0 1 1h16a1 1 0 0 0 1-1V4a1 1 0 0 0-1-1z'
  },
  subscriptions: {
    regular: 'M19 8H5V6h14v2zm0 5H5v-2h14v2zm0 5H5v-2h14v2zM3 4h18v16H3V4zm2 2v12h14V6H5z',
    filled: 'M3 4h18v16H3V4zm4 4h10v2H7V8zm0 4h10v2H7v-2zm0 4h7v2H7v-2z'
  },
  settings: {
    regular: 'M19.43 12.98c.04-.32.07-.64.07-.98s-.03-.66-.07-.98l2.11-1.65-.12-.64-2-3.46-.61-.22-2.49 1a7.4 7.4 0 0 0-1.69-.98l-.38-2.65L14 2h-4l-.49.42-.38 2.65c-.61.25-1.17.58-1.69.98l-2.49-1-.61.22-2 3.46-.12.64 2.11 1.65c-.04.32-.07.65-.07.98s.03.66.07.98l-2.11 1.65.12.64 2 3.46.61.22 2.49-1c.52.4 1.08.73 1.69.98l.38 2.65L10 22h4l.49-.42.38-2.65c.61-.25 1.17-.58 1.69-.98l2.49 1 .61-.22 2-3.46-.12-.64-2.11-1.65zM12 15.5A3.5 3.5 0 1 1 12 8a3.5 3.5 0 0 1 0 7.5z',
    filled: 'M19.43 12.98c.04-.32.07-.64.07-.98s-.03-.66-.07-.98l2.11-1.65-.12-.64-2-3.46-.61-.22-2.49 1a7.4 7.4 0 0 0-1.69-.98l-.38-2.65L14 2h-4l-.49.42-.38 2.65c-.61.25-1.17.58-1.69.98l-2.49-1-.61.22-2 3.46-.12.64 2.11 1.65c-.04.32-.07.65-.07.98s.03.66.07.98l-2.11 1.65.12.64 2 3.46.61.22 2.49-1c.52.4 1.08.73 1.69.98l.38 2.65L10 22h4l.49-.42.38-2.65c.61-.25 1.17-.58 1.69-.98l2.49 1 .61-.22 2-3.46-.12-.64-2.11-1.65zM12 16a4 4 0 1 1 0-8 4 4 0 0 1 0 8z'
  }
};

const selectTab = (tab: TabId) => {
  if (tab === activeTab.value) return;
  if (tab === 'dashboard') router.back();
  else if (activeTab.value === 'dashboard') router.push(routes[tab]);
  else router.replace(routes[tab]);
};

const viewportHeight = () => window.visualViewport?.height ?? window.innerHeight;
const onViewportResize = () => {
  const height = viewportHeight();
  if (height > viewportBaseline) {
    viewportBaseline = height;
    keyboardOpen.value = false;
    return;
  }
  keyboardOpen.value = viewportBaseline - height > 150;
};

onMounted(() => {
  setEdgeToEdge(true);
  viewportBaseline = viewportHeight();
  window.visualViewport?.addEventListener('resize', onViewportResize);
  window.addEventListener('resize', onViewportResize);
});
onUnmounted(() => {
  window.visualViewport?.removeEventListener('resize', onViewportResize);
  window.removeEventListener('resize', onViewportResize);
});
</script>

<template>
  <div class="app-root">
    <header v-if="!childPage" class="top-bar"><h1>{{ screenTitle }}</h1></header>
    <main class="main-content" :style="{ height: mainHeight }">
      <div class="page-scroller">
        <router-view v-slot="{ Component }">
          <keep-alive include="DashboardScreen,NodesScreen,SubscriptionsScreen,SettingsLayout">
            <component :is="Component" @navigate="selectTab" />
          </keep-alive>
        </router-view>
      </div>
    </main>
    <nav v-if="!hideBottomNav" class="bottom-nav">
      <button v-for="tab in tabOrder" :key="tab" type="button" :class="['nav-tab', { active: activeTab === tab }]" @click="selectTab(tab)">
        <span class="icon-container"><md-icon><svg viewBox="0 0 24 24"><path :d="activeTab === tab ? icons[tab].filled : icons[tab].regular" /></svg></md-icon></span>
        <span class="label">{{ t(`nav.${tab}`) }}</span>
      </button>
    </nav>
  </div>
</template>

<style scoped>
.top-bar { height: calc(64px + var(--top-inset)); padding: var(--top-inset) 20px 0; box-sizing: border-box; display: flex; align-items: center; color: var(--md-sys-color-on-surface); background: var(--md-sys-color-background); }
.top-bar h1 { margin: 0; font-size: 22px; }
.bottom-nav { position: fixed; inset: auto 0 0; z-index: 100; height: calc(var(--bottom-nav-height) + var(--bottom-inset)); padding-bottom: var(--bottom-inset); box-sizing: border-box; display: flex; align-items: center; background: var(--md-sys-color-surface-container-high); }
.nav-tab { flex: 1; height: 100%; border: 0; padding: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 2px; font-size: 11px; color: var(--md-sys-color-on-surface-variant); background: transparent; }
.icon-container { width: 32px; height: 32px; border-radius: 8px; display: grid; place-items: center; transition: width .2s, background-color .2s; }
.nav-tab.active .icon-container { width: 54px; color: var(--md-sys-color-on-secondary-container); background: var(--md-sys-color-secondary-container); }
.nav-tab.active { color: var(--md-sys-color-on-surface); font-weight: 600; }
md-icon, md-icon svg { width: 22px; height: 22px; display: block; fill: currentColor; }
</style>
