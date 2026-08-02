<script setup lang="ts">
/**
 * @file AppsScreen.vue
 * @description 应用页：分应用代理（关闭/黑名单/白名单）管理。列出已安装应用、搜索/过滤/排序、
 *   勾选加入名单（写 ebpf.conf）、下拉刷新；长列表用自建虚拟滚动（复用父级 .page-scroller）。
 *   非真机环境用 mock 数据（含约 300 条用于验证虚拟滚动）。
 */
import { ref, computed, onMounted, onActivated, onDeactivated, onUnmounted, nextTick, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import {
  showToast,
  getAppPackagesList,
  getAppIconUrl,
  isKsuEnv,
  writeEbpfValue,
  addProxyApp,
  removeProxyApp,
  getEbpfConfigState,
  runCli
} from '../utils/ksu';
import { useBackDismiss } from '../composables/useBackDismiss';

const { t } = useI18n();

interface AppItem {
  packageName: string;
  versionName: string;
  versionCode: number;
  appLabel: string;
  isSystem: boolean;
  uid: number;
  iconLetter: string;
  checked: boolean;
  iconColor: string;     // 预计算的头像底色 (避免每渲染哈希)
  userId: string;        // 预计算的用户 ID
  showUserBadge: boolean; // 是否显示 USER 徽章 (userId !== '0')
}

const apps = ref<AppItem[]>([]);
const isLoading = ref(false);
const searchQuery = ref('');

// 真机环境用 ksu://icon 真实图标，浏览器/加载失败回退字母头像
const isKsu = isKsuEnv();
const iconFailed = ref<Set<string>>(new Set());
/** 图标加载失败回调：记录该包名，后续回退字母头像。 */
const onIconError = (packageName: string) => {
  iconFailed.value = new Set(iconFailed.value).add(packageName);
};

// 分应用代理偏好状态
const isAppProxyEnabled = ref(false);
const appProxyMode = ref<'blacklist' | 'whitelist'>('blacklist');
const isApplyingConfig = ref(false);

// 展示过滤选项（持久化到 localStorage）
const showSystemApps = ref(localStorage.getItem('np_show_system_apps') === 'true');
const appSelectedFirst = ref(localStorage.getItem('np_app_selected_first') !== 'false'); // 默认 true
const appReverseSort = ref(localStorage.getItem('np_app_reverse_sort') === 'true');
const appShowPackageName = ref(localStorage.getItem('np_app_show_package_name') === 'true');

// 下拉刷新状态
const pullDelta = ref(0);
const isRefreshing = ref(false);
const refreshText = computed(() => {
  if (isRefreshing.value) return t('nodes.refreshing');
  if (pullDelta.value > 50) return t('nodes.releaseToRefresh');
  return t('nodes.pullToRefresh');
});

let touchStartY = 0;
let isPulling = false;

// 设置弹窗
const showSettingsDialog = ref(false);

// 手势返回优先关闭设置弹窗
useBackDismiss(
  () => showSettingsDialog.value,
  () => { showSettingsDialog.value = false; }
);

/** 打开「显示设置」弹窗（供顶栏菜单按钮调用）。 */
const openAppsMenu = () => {
  showSettingsDialog.value = true;
};

// md-dialog 关闭时，原生 <dialog> 会把焦点还原到打开它的元素（右上角三点菜单按钮），
// 使该按钮出现 focus-visible 选中圈。关闭动画结束后主动失焦，消除这一残留高亮。
// (打开时第一个开关的选中圈由模板中的 [autofocus] 哨兵元素接管，故无需再处理 open。)
const onSettingsDialogClosed = () => {
  const active = document.activeElement as HTMLElement | null;
  if (active && typeof active.blur === 'function') active.blur();
};

// 持久化各展示偏好的 setter
const setShowSystemApps = (val: boolean) => {
  showSystemApps.value = val;
  localStorage.setItem('np_show_system_apps', String(val));
};
const setAppSelectedFirst = (val: boolean) => {
  appSelectedFirst.value = val;
  localStorage.setItem('np_app_selected_first', String(val));
};
const setAppReverseSort = (val: boolean) => {
  appReverseSort.value = val;
  localStorage.setItem('np_app_reverse_sort', String(val));
};
const setAppShowPackageName = (val: boolean) => {
  appShowPackageName.value = val;
  localStorage.setItem('np_app_show_package_name', String(val));
};

/**
 * 由包名哈希出确定性的头像底色（HSL）。
 * @param pkgName  包名
 * @returns CSS 颜色串
 */
const getIconColor = (pkgName: string) => {
  let hash = 0;
  for (let i = 0; i < pkgName.length; i++) {
    hash = pkgName.charCodeAt(i) + ((hash << 5) - hash);
  }
  const h = Math.abs(hash % 360);
  return `hsl(${h}, 60%, 55%)`; // 柔和色调
};

/**
 * 从 Android UID 推出用户 ID（uid / 100000）。
 * @param uid  应用 UID
 * @returns 用户 ID 字符串
 */
const getUserId = (uid: number): string => {
  return String(Math.floor(uid / 100000));
};

// 下拉选中下标：0=关闭 1=黑名单 2=白名单
const proxyModeIndex = computed(() => {
  if (!isAppProxyEnabled.value) return 0;
  return appProxyMode.value === 'whitelist' ? 2 : 1;
});

// ===================================================================
// 配置加载 / 应用列表加载
// ===================================================================

/** 经共享 ebpf.conf 读取器加载分应用代理配置（真机/mock 双轨），并据名单标记勾选态。 */
const loadAppConfig = async () => {
  try {
    const state = await getEbpfConfigState();
    isAppProxyEnabled.value = state.appProxyEnabled;
    appProxyMode.value = state.appProxyMode;

    // proxiedAppItems 已是当前模式对应的列表，按 userId:packageName 或裸包名匹配选中态
    const checkedSet = new Set(state.proxiedAppItems);
    apps.value.forEach(app => {
      const userId = getUserId(app.uid);
      const targetWithUser = `${userId}:${app.packageName}`;
      app.checked = checkedSet.has(targetWithUser) || checkedSet.has(app.packageName);
    });
  } catch (e) {
    console.error('Failed to load eBPF config:', e);
  }
};

/** 加载已安装应用列表并标记勾选态；真机失败时回退含约 300 条的 mock 数据集。 */
const loadPackages = async () => {
  isLoading.value = true;
  try {
    const pkgs = await getAppPackagesList('all');
    if (pkgs && pkgs.length > 0) {
      apps.value = pkgs.map(pkg => {
        const userId = getUserId(pkg.uid);
        return {
          packageName: pkg.packageName,
          versionName: pkg.versionName,
          versionCode: pkg.versionCode,
          appLabel: pkg.appLabel || pkg.packageName,
          isSystem: pkg.isSystem,
          uid: pkg.uid,
          iconLetter: pkg.appLabel ? pkg.appLabel.charAt(0).toUpperCase() : '?',
          checked: false,
          iconColor: getIconColor(pkg.packageName),
          userId,
          showUserBadge: userId !== '0'
        };
      });
    } else {
      throw new Error('Empty packages returned');
    }
    await loadAppConfig();
  } catch (err: any) {
    console.warn('Failed to load native packages list, falling back to mock dataset:', err);
    // 基础 mock + 批量生成，凑到 ~300 个用于验证虚拟滚动
    const baseMock = [
      { packageName: 'org.telegram.messenger', appLabel: 'Telegram', isSystem: false, uid: 10243, checked: true },
      { packageName: 'com.google.android.youtube', appLabel: 'YouTube', isSystem: false, uid: 10242, checked: true },
      { packageName: 'com.tencent.mm', appLabel: '微信 (WeChat)', isSystem: false, uid: 10241, checked: true },
      { packageName: 'com.github.android', appLabel: 'GitHub', isSystem: false, uid: 10244, checked: false },
      { packageName: 'com.netflix.mediaclient', appLabel: 'Netflix', isSystem: false, uid: 10245, checked: false },
      { packageName: 'com.android.chrome', appLabel: 'Chrome 浏览器', isSystem: true, uid: 10045, checked: false },
      { packageName: 'com.android.vending', appLabel: 'Google Play 商店', isSystem: true, uid: 10012, checked: false },
      { packageName: 'com.eg.android.AlipayGphone', appLabel: '支付宝 (Alipay)', isSystem: false, uid: 10246, checked: false },
      { packageName: 'com.android.settings', appLabel: '系统设置', isSystem: true, uid: 10000, checked: false }
    ];
    for (let i = 1; i <= 291; i++) {
      baseMock.push({
        packageName: `com.example.app${i}`,
        appLabel: `示例应用 ${String(i).padStart(3, '0')}`,
        isSystem: i % 4 === 0,
        uid: 10300 + i,
        checked: false
      });
    }
    apps.value = baseMock.map(m => {
      const userId = getUserId(m.uid);
      return {
        packageName: m.packageName,
        versionName: '1.0',
        versionCode: 1,
        appLabel: m.appLabel,
        isSystem: m.isSystem,
        uid: m.uid,
        iconLetter: m.appLabel.charAt(0).toUpperCase(),
        checked: m.checked,
        iconColor: getIconColor(m.packageName),
        userId,
        showUserBadge: userId !== '0'
      };
    });
    // mock 模式下从 localStorage 恢复勾选态（若有）
    const mockMode = localStorage.getItem('mock_proxy_mode') || 'blacklist';
    const mockEnabled = localStorage.getItem('mock_proxy_enabled') !== 'false';
    isAppProxyEnabled.value = mockEnabled;
    appProxyMode.value = mockMode as 'blacklist' | 'whitelist';

    // 从存储同步勾选态
    const storedChecked = localStorage.getItem('mock_checked_apps');
    if (storedChecked) {
      const checkedSet = new Set(JSON.parse(storedChecked));
      apps.value.forEach(app => {
        app.checked = checkedSet.has(app.packageName);
      });
    }
  } finally {
    isLoading.value = false;
  }
};

// ===================================================================
// 下拉刷新触摸处理
// ===================================================================

/** 触摸开始：仅在列表顶部、无弹窗、未刷新时进入下拉判定。 */
const handleTouchStart = (e: TouchEvent) => {
  // 弹窗打开时不触发下拉刷新，避免与弹窗内滚动/手势冲突
  if (showSettingsDialog.value) return;
  const container = document.querySelector('.page-scroller');
  if (!container || container.scrollTop > 0 || isRefreshing.value) return;
  touchStartY = e.touches[0].clientY;
  isPulling = true;
};

/** 触摸移动：按阻尼累计下拉距离，超过阈值时阻止默认滚动。 */
const handleTouchMove = (e: TouchEvent) => {
  if (!isPulling || isRefreshing.value) return;
  const currentY = e.touches[0].clientY;
  const delta = currentY - touchStartY;
  if (delta > 0) {
    pullDelta.value = Math.min(80, delta * 0.45);
    if (pullDelta.value > 0) {
      e.preventDefault();
    }
  }
};

/** 触摸结束：下拉超过阈值则触发刷新，否则回弹。 */
const handleTouchEnd = async () => {
  if (!isPulling) return;
  isPulling = false;
  if (pullDelta.value > 50) {
    isRefreshing.value = true;
    pullDelta.value = 50;
    try {
      await loadPackages();
      showToast(t('apps.refreshed'));
    } catch (err) {
      // 忽略
    } finally {
      isRefreshing.value = false;
      pullDelta.value = 0;
    }
  } else {
    pullDelta.value = 0;
  }
};

// ===================================================================
// 代理模式切换 / 应用勾选
// ===================================================================

/**
 * 代理模式下拉变更（0=关闭 1=黑名单 2=白名单）：写 ebpf.conf 并刷新配置。
 * @param e  select 的 change 事件
 */
const handleProxyModeIndexChange = async (e: Event) => {
  const select = e.target as HTMLSelectElement;
  const index = parseInt(select.value);
  try {
    if (isKsuEnv()) {
      if (index === 0) {
        await writeEbpfValue('APP_PROXY_ENABLE', '0');
        isAppProxyEnabled.value = false;
        showToast(t('apps.proxyDisabled'));
      } else {
        await writeEbpfValue('APP_PROXY_ENABLE', '1');
        isAppProxyEnabled.value = true;
        const mode = index === 1 ? 'blacklist' : 'whitelist';
        await writeEbpfValue('APP_PROXY_MODE', mode, true);
        appProxyMode.value = mode;
        showToast(t('apps.modeChanged', { mode: mode === 'blacklist' ? t('apps.blacklist') : t('apps.whitelist') }));
      }
      await loadAppConfig();
    } else {
      // mock 环境持久化
      if (index === 0) {
        isAppProxyEnabled.value = false;
        localStorage.setItem('mock_proxy_enabled', 'false');
        showToast(t('apps.proxyDisabled'));
      } else {
        isAppProxyEnabled.value = true;
        localStorage.setItem('mock_proxy_enabled', 'true');
        const mode = index === 1 ? 'blacklist' : 'whitelist';
        appProxyMode.value = mode;
        localStorage.setItem('mock_proxy_mode', mode);
        showToast(t('apps.modeChanged', { mode: mode === 'blacklist' ? t('apps.blacklist') : t('apps.whitelist') }));
      }
    }
  } catch (err: any) {
    showToast(t('apps.switchProxyFailed', { msg: err.message || err }));
  }
};

/** 重启 sing-box，使连续修改的分应用名单一次性生效。 */
const applyAppProxyConfig = async () => {
  if (isApplyingConfig.value) return;
  isApplyingConfig.value = true;
  try {
    await runCli('service restart', { detach: true });
    showToast(t('apps.applySuccess'));
  } catch (err: any) {
    showToast(t('apps.applyFailed', { msg: err?.message || String(err) }));
  } finally {
    isApplyingConfig.value = false;
  }
};

/**
 * 切换某应用的代理勾选态（乐观更新 → 写名单），失败回滚。
 * @param app  目标应用
 */
const toggleAppProxy = async (app: AppItem) => {
  const originalState = app.checked;
  app.checked = !originalState;
  
  const userId = getUserId(app.uid);
  
  try {
    if (isKsuEnv()) {
      if (originalState) {
        await removeProxyApp(app.packageName, userId);
        showToast(t('apps.removedFromList', { label: app.appLabel }));
      } else {
        await addProxyApp(app.packageName, userId);
        showToast(t('apps.addedToList', { label: app.appLabel }));
      }
      await loadAppConfig();
    } else {
      // mock：保存勾选态到 localStorage
      const checkedApps = apps.value.filter(a => a.checked).map(a => a.packageName);
      localStorage.setItem('mock_checked_apps', JSON.stringify(checkedApps));
      showToast(app.checked ? t('apps.addedToList', { label: app.appLabel }) : t('apps.removedFromList', { label: app.appLabel }));
    }
  } catch (err: any) {
    // 出错时回滚勾选态
    app.checked = originalState;
    showToast(t('apps.updateConfigFailed', { msg: err.message || err }));
  }
};

// ===================================================================
// 过滤 / 排序
// ===================================================================

/** 按「系统应用过滤 → 搜索 → 排序（可选选中优先 + 本地化名称，可反序）」派生展示列表。 */
const filteredApps = computed(() => {
  // 1. 系统应用过滤
  let result = apps.value.filter(app => {
    if (!showSystemApps.value && app.isSystem) return false;
    return true;
  });

  // 2. 搜索过滤
  if (searchQuery.value.trim()) {
    const q = searchQuery.value.toLowerCase().trim();
    result = result.filter(app => 
      app.appLabel.toLowerCase().includes(q) || 
      app.packageName.toLowerCase().includes(q)
    );
  }

  // 3. 排序逻辑
  result.sort((a, b) => {
    // 3a. 选中项优先
    if (appSelectedFirst.value) {
      if (a.checked && !b.checked) return -1;
      if (!a.checked && b.checked) return 1;
    }

    // 3b. 按名称本地化排序
    const comp = a.appLabel.localeCompare(b.appLabel, 'zh-CN');
    return appReverseSort.value ? -comp : comp;
  });

  return result;
});

// ===== 虚拟滚动引擎 (复用父级 .page-scroller，单列等高) =====
const listEl = ref<HTMLElement | null>(null);
let scrollerEl: HTMLElement | null = null;
const scrollTop = ref(0);
const viewportH = ref(0);
const listTop = ref(0);
const measuredRowH = ref(0);
const VS_OVERSCAN = 6;
const ROW_GAP = 10;  // 与 .app-item-card 的 margin-bottom 一致 (offsetHeight 不含 margin)

// 行高估算(含行间距)：含包名副标题时更高；实测后用 measuredRowH 校准
const estimatedRowH = computed(() => (appShowPackageName.value ? 76 : 60) + ROW_GAP);
const rowHeight = computed(() => measuredRowH.value || estimatedRowH.value);

const totalHeight = computed(() => filteredApps.value.length * rowHeight.value);

/** 据滚动位置与行高算出应渲染的切片范围（含上下 overscan）。 */
const visibleRange = computed(() => {
  const rh = rowHeight.value;
  const n = filteredApps.value.length;
  if (rh <= 0 || n === 0) return { start: 0, end: 0, offsetY: 0 };
  const relTop = scrollTop.value - listTop.value;
  let start = Math.floor(relTop / rh) - VS_OVERSCAN;
  if (start < 0) start = 0;
  const visible = Math.ceil(viewportH.value / rh) + VS_OVERSCAN * 2;
  let end = start + visible;
  if (end > n) end = n;
  return { start, end, offsetY: start * rh };
});

/** 当前应渲染的应用切片。 */
const visibleApps = computed(() => {
  const { start, end } = visibleRange.value;
  return filteredApps.value.slice(start, end);
});

let vsRaf = 0;
/** 滚动/尺寸变化时（rAF 节流）更新滚动位置、视口高与列表顶偏移。 */
const onScroll = () => {
  if (vsRaf) return;
  vsRaf = requestAnimationFrame(() => {
    vsRaf = 0;
    if (!scrollerEl) return;
    scrollTop.value = scrollerEl.scrollTop;
    viewportH.value = scrollerEl.clientHeight;
    if (listEl.value) {
      const lr = listEl.value.getBoundingClientRect();
      const sr = scrollerEl.getBoundingClientRect();
      listTop.value = (lr.top - sr.top) + scrollerEl.scrollTop;
    }
  });
};

/** 实测首个卡片行高（含行间距）校准估算值。 */
const measureRow = () => {
  if (!listEl.value) return;
  const card = listEl.value.querySelector('.app-item-card') as HTMLElement | null;
  if (card) {
    const h = card.offsetHeight + ROW_GAP;  // 行距 = 卡片高 + margin-bottom
    if (h > 0 && Math.abs(h - measuredRowH.value) > 1) measuredRowH.value = h;
  }
};

let vsResizeObserver: ResizeObserver | null = null;
/** 挂载滚动监听与 ResizeObserver（复用父级 .page-scroller）。 */
const attachScroll = () => {
  scrollerEl = document.querySelector('.page-scroller');
  if (scrollerEl) {
    scrollerEl.addEventListener('scroll', onScroll, { passive: true });
    viewportH.value = scrollerEl.clientHeight;
    scrollTop.value = scrollerEl.scrollTop;
  }
  if (typeof ResizeObserver !== 'undefined') {
    vsResizeObserver = new ResizeObserver(() => { onScroll(); measureRow(); });
    if (scrollerEl) vsResizeObserver.observe(scrollerEl);
  }
  nextTick(() => { onScroll(); measureRow(); });
};
/** 卸载滚动监听、ResizeObserver 与待执行的 rAF。 */
const detachScroll = () => {
  if (scrollerEl) scrollerEl.removeEventListener('scroll', onScroll);
  if (vsResizeObserver) { vsResizeObserver.disconnect(); vsResizeObserver = null; }
  if (vsRaf) { cancelAnimationFrame(vsRaf); vsRaf = 0; }
};

onActivated(attachScroll);
onDeactivated(detachScroll);
onUnmounted(detachScroll);

// 列表/行高变化后重新定位与实测
watch([filteredApps, appShowPackageName, searchQuery], () => {
  nextTick(() => { onScroll(); measureRow(); });
});

onMounted(async () => {
  await loadPackages();
});

defineExpose({
  openAppsMenu
});
</script>

<template>
  <div 
    class="page-container animated-fade-in"
    @touchstart="handleTouchStart"
    @touchmove="handleTouchMove"
    @touchend="handleTouchEnd"
  >
    <!-- 下拉刷新指示器 -->
    <div 
      class="pull-to-refresh-indicator" 
      v-show="pullDelta > 0 || isRefreshing"
      :style="{ height: pullDelta + 'px', opacity: pullDelta > 0 || isRefreshing ? 1 : 0 }"
      :class="{ 'refreshing': isRefreshing }"
    >
      <div class="refresh-spinner" v-if="isRefreshing">
        <svg class="spinner-svg" viewBox="0 0 24 24">
          <circle cx="12" cy="12" r="10" fill="none" stroke="currentColor" stroke-width="3"></circle>
        </svg>
      </div>
      <div class="refresh-arrow" v-else :style="{ transform: `rotate(${Math.min(pullDelta * 3.6, 180)}deg)` }">↓</div>
      <span class="refresh-label">{{ refreshText }}</span>
    </div>

    <!-- 顶部搜索栏（pill 样式） -->
    <div class="search-bar-row">
      <div class="search-bar">
        <svg class="search-icon" viewBox="0 0 24 24">
          <path d="M15.5 14h-.79l-.28-.27C15.41 12.59 16 11.11 16 9.5 16 5.91 13.09 3 9.5 3S3 5.91 3 9.5 5.91 16 9.5 16c1.61 0 3.09-.59 4.23-1.57l.27.28v.79l5 4.99L20.49 19l-4.99-5zm-6 0C7.01 14 5 11.99 5 9.5S7.01 5 9.5 5 14 7.01 14 9.5 11.99 14 9.5 14z"/>
        </svg>
        <input
          type="text"
          class="search-input"
          v-model="searchQuery"
          :placeholder="t('apps.searchPlaceholder')"
          :aria-label="t('apps.searchPlaceholder')" />
      </div>
    </div>

    <!-- 分应用代理模式选择卡片 -->
    <div class="config-card app-pref-card">
      <div class="pref-row">
        <div class="pref-text">
          <span class="pref-title">{{ t('apps.appProxy') }}</span>
          <span class="pref-summary">{{ t('apps.appProxyDesc') }}</span>
        </div>
        <select :value="proxyModeIndex" @change="handleProxyModeIndexChange($event)" class="pref-dropdown">
          <option :value="0">{{ t('apps.modeOff') }}</option>
          <option :value="1">{{ t('apps.modeBlacklist') }}</option>
          <option :value="2">{{ t('apps.modeWhitelist') }}</option>
        </select>
      </div>
      <div class="app-pref-divider"></div>
      <div class="pref-row apply-pref-row">
        <div class="pref-text">
          <span class="pref-title">{{ t('apps.applyChanges') }}</span>
          <span class="pref-summary">{{ t('apps.applyChangesDesc') }}</span>
        </div>
        <md-filled-button :disabled="isApplyingConfig" @click="applyAppProxyConfig">
          {{ isApplyingConfig ? t('apps.applying') : t('apps.applyChanges') }}
        </md-filled-button>
      </div>
    </div>

    <!-- 应用列表：直接平铺，随父级 page-scroller 滚动 -->
    <div class="apps-flow-list">
      <!-- 加载指示（仅首次加载） -->
      <div v-if="isLoading && apps.length === 0" class="loading-state">
        <md-circular-progress indeterminate></md-circular-progress>
      </div>

      <!-- 空状态 -->
      <div v-else-if="filteredApps.length === 0" class="list-empty">
        <span>{{ t('apps.notFound') }}</span>
      </div>

      <!-- 虚拟滚动：外层撑高，内层只渲染可见切片并按偏移定位 -->
      <div v-else ref="listEl" class="apps-virtual" :style="{ height: totalHeight + 'px' }">
        <div class="apps-virtual-inner" :style="{ transform: `translateY(${visibleRange.offsetY}px)` }">
          <div
            v-for="app in visibleApps"
            :key="app.packageName + app.uid"
            class="app-item-card"
            @click="toggleAppProxy(app)">

            <div class="app-item-start">
              <div class="app-icon-avatar" :style="{ backgroundColor: app.iconColor }">
                <img
                  v-if="isKsu && !iconFailed.has(app.packageName)"
                  class="app-icon-img"
                  :src="getAppIconUrl(app.packageName)"
                  alt=""
                  loading="lazy"
                  @error="onIconError(app.packageName)" />
                <span v-else>{{ app.iconLetter }}</span>
              </div>
            </div>

            <div class="app-item-content">
              <div class="app-item-title-wrapper">
                <span class="app-item-title">{{ app.appLabel }}</span>
              </div>
              <div v-if="appShowPackageName" class="app-item-subtitle-wrapper">
                <span class="app-item-subtitle">{{ app.packageName }}</span>
              </div>
            </div>

            <div class="app-item-end">
              <div v-if="app.showUserBadge" class="user-id-badge">
                USER {{ app.userId }}
              </div>
              <md-checkbox
                :checked="app.checked"
                @click.stop="toggleAppProxy(app)">
              </md-checkbox>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 显示偏好设置弹窗 -->
    <div class="dialogs-wrapper">
      <md-dialog :open="showSettingsDialog" @close="showSettingsDialog = false" @closed="onSettingsDialogClosed" class="transparent-scrim">
        <div slot="headline">{{ t('apps.displaySettings') }}</div>
        <div slot="content" class="display-dialog-content">
          <!-- autofocus 哨兵：md-dialog 打开时会聚焦首个可聚焦子元素，让此不可见元素
               接管初始焦点，避免第一个开关出现 focus-visible 选中圈 -->
          <span tabindex="-1" autofocus aria-hidden="true" class="focus-sink"></span>
          <div class="preference-group-card">
            
            <div class="switch-pref-row" @click="setShowSystemApps(!showSystemApps)">
              <div class="dropdown-text">
                <span class="dropdown-title">{{ t('apps.showSystemApps') }}</span>
              </div>
              <md-switch :selected="showSystemApps" @click.stop="setShowSystemApps(!showSystemApps)"></md-switch>
            </div>
            
            <div class="pref-inner-divider"></div>
            
            <div class="switch-pref-row" @click="setAppSelectedFirst(!appSelectedFirst)">
              <div class="dropdown-text">
                <span class="dropdown-title">{{ t('apps.selectedFirst') }}</span>
              </div>
              <md-switch :selected="appSelectedFirst" @click.stop="setAppSelectedFirst(!appSelectedFirst)"></md-switch>
            </div>

            <div class="pref-inner-divider"></div>
            
            <div class="switch-pref-row" @click="setAppReverseSort(!appReverseSort)">
              <div class="dropdown-text">
                <span class="dropdown-title">{{ t('apps.reverseSort') }}</span>
              </div>
              <md-switch :selected="appReverseSort" @click.stop="setAppReverseSort(!appReverseSort)"></md-switch>
            </div>

            <div class="pref-inner-divider"></div>
            
            <div class="switch-pref-row" @click="setAppShowPackageName(!appShowPackageName)">
              <div class="dropdown-text">
                <span class="dropdown-title">{{ t('apps.showPackageName') }}</span>
              </div>
              <md-switch :selected="appShowPackageName" @click.stop="setAppShowPackageName(!appShowPackageName)"></md-switch>
            </div>
            
          </div>
        </div>
        <div slot="actions">
          <md-text-button @click="showSettingsDialog = false">{{ t('common.done') }}</md-text-button>
        </div>
      </md-dialog>
    </div>
  </div>
</template>

<style scoped>
.page-container {
  display: flex;
  flex-direction: column;
  width: 100%;
}

/* 下拉刷新样式 */
.pull-to-refresh-indicator {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  overflow: hidden;
  transition: height 0.15s cubic-bezier(0.3, 1, 0.2, 1), opacity 0.15s ease;
  width: 100%;
  color: var(--md-sys-color-primary);
  font-size: 13px;
  font-weight: 500;
  user-select: none;
  pointer-events: none;
  margin-bottom: 8px;
}

.refresh-arrow {
  font-size: 16px;
  font-weight: bold;
  transition: transform 0.15s ease;
}

.refresh-spinner {
  display: flex;
  align-items: center;
  justify-content: center;
}

.spinner-svg {
  width: 18px;
  height: 18px;
  animation: rotateSpinner 0.8s linear infinite;
}

.spinner-svg circle {
  stroke-dasharray: 40;
  stroke-dashoffset: 0;
  transform-origin: center;
  animation: dashSpinner 1.5s ease-in-out infinite;
  stroke: currentColor;
}

@keyframes rotateSpinner {
  100% {
    transform: rotate(360deg);
  }
}

@keyframes dashSpinner {
  0% {
    stroke-dashoffset: 40;
    transform: rotate(0deg);
  }
  50% {
    stroke-dashoffset: 10;
    transform: rotate(180deg);
  }
  100% {
    stroke-dashoffset: 40;
    transform: rotate(360deg);
  }
}

/* 搜索栏样式（pill 样式） */
.search-bar-row {
  display: flex;
  align-items: center;
  margin-bottom: 12px;
  width: 100%;
}

.search-bar {
  flex: 1;
  background-color: var(--md-sys-color-surface-container-high);
  border-radius: var(--radius-2xl);
  height: 48px;
  display: flex;
  align-items: center;
  padding: 0 16px;
  gap: 12px;
  transition: background-color 0.2s;
}

.search-bar:focus-within {
  background-color: var(--md-sys-color-surface-container-highest);
}

.search-icon {
  width: 20px;
  height: 20px;
  fill: var(--md-sys-color-on-surface);
  opacity: 0.7;
  flex-shrink: 0;
}

.search-input {
  flex: 1;
  border: none;
  background: transparent;
  height: 100%;
  font-size: 14px;
  color: var(--md-sys-color-on-surface);
  outline: none;
  min-width: 0;
  padding: 0;
}

.search-input::placeholder {
  color: var(--md-sys-color-on-surface-variant);
  opacity: 0.7;
}

/* 偏好卡片样式 */
.app-pref-card {
  margin-bottom: 14px;
  gap: 0;
}

.pref-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  width: 100%;
}

.apply-pref-row {
  padding-top: 14px;
}

.apply-pref-row md-filled-button {
  flex-shrink: 0;
}

.app-pref-divider {
  height: 1px;
  margin: 14px 0 0;
  background-color: var(--md-sys-color-outline-variant);
}

.pref-text {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
  flex-grow: 1;
  padding-right: 12px;
}

.pref-title {
  font-size: 16px;
  font-weight: 550;
  color: var(--md-sys-color-on-surface);
}

.pref-summary {
  font-size: 12.5px;
  color: var(--md-sys-color-on-surface-variant);
  text-overflow: ellipsis;
  overflow: hidden;
  white-space: nowrap;
}

.pref-dropdown {
  background-color: var(--md-sys-color-surface-container-high);
  color: var(--md-sys-color-on-surface);
  border: 1px solid var(--md-sys-color-outline-variant);
  padding: 8px 12px;
  border-radius: var(--radius-sm);
  outline: none;
  font-size: 13.5px;
  font-weight: 500;
  cursor: pointer;
  appearance: none;
}

/* 平铺应用列表 */
.apps-flow-list {
  display: flex;
  flex-direction: column;
  width: 100%;
}

/* 虚拟滚动容器：外层撑高，内层绝对定位承载可见切片 */
.apps-virtual {
  position: relative;
  width: 100%;
}
.apps-virtual-inner {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  display: flex;
  flex-direction: column;
  will-change: transform;
}

.loading-state {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 48px 0;
}

.list-empty {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 48px 0;
  color: var(--md-sys-color-on-surface-variant);
  font-size: 14px;
}

/* 应用卡片项样式 */
.app-item-card {
  background-color: var(--md-sys-color-surface-container);
  border: 1px solid var(--md-sys-color-outline-variant);
  border-radius: var(--radius-lg);
  padding: 12px 16px;
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 10px;
  cursor: pointer;
  transition: background-color 0.2s, transform 0.2s;
  user-select: none;
  width: 100%;
}

.app-item-card:active {
  transform: scale(0.985);
  background-color: var(--md-sys-color-surface-container-high);
}

.app-item-start {
  width: 40px;
  height: 40px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.app-icon-avatar {
  width: 40px;
  height: 40px;
  border-radius: 10px;
  color: #ffffff;
  font-weight: bold;
  font-size: 18px;
  display: flex;
  align-items: center;
  justify-content: center;
  user-select: none;
  overflow: hidden;
  box-shadow: 0 2px 4px rgba(0, 0, 0, 0.1);
}

/* 真实应用图标：铺满圆角头像 */
.app-icon-img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}

.app-item-content {
  flex-grow: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  justify-content: center;
}

.app-item-title-wrapper {
  overflow: hidden;
  white-space: nowrap;
}

.app-item-title {
  font-size: 16px;
  font-weight: 550;
  color: var(--md-sys-color-on-background);
  display: inline-block;
  max-width: 100%;
  text-overflow: ellipsis;
  overflow: hidden;
}

.app-item-subtitle-wrapper {
  overflow: hidden;
  white-space: nowrap;
  margin-top: 2px;
}

.app-item-subtitle {
  font-size: 12px;
  font-weight: 500;
  color: var(--md-sys-color-on-surface-variant);
  display: inline-block;
  max-width: 100%;
  text-overflow: ellipsis;
  overflow: hidden;
  font-family: var(--md-ref-typeface-mono);
}

.app-item-end {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-shrink: 0;
}

.user-id-badge {
  background-color: var(--md-sys-color-secondary-container);
  color: var(--md-sys-color-on-secondary-container);
  font-size: 10px;
  font-weight: 600;
  padding: 3px 6px;
  border-radius: var(--radius-xs);
  user-select: none;
  white-space: nowrap;
  opacity: 0.9;
}

/* 弹窗内显示偏好卡片样式 */
.display-dialog-content {
  display: flex;
  flex-direction: column;
  gap: 12px;
  margin-top: 8px;
}

/* autofocus 哨兵：不占空间、不显示、无焦点轮廓，仅用于接管弹窗初始焦点 */
.focus-sink {
  position: absolute;
  width: 0;
  height: 0;
  overflow: hidden;
  outline: none;
}

.preference-group-card {
  background-color: var(--md-sys-color-surface-container);
  border-radius: var(--radius-xl);
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border: 1px solid var(--md-sys-color-outline-variant);
}

.switch-pref-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 14px 16px;
  width: 100%;
  box-sizing: border-box;
  cursor: pointer;
  transition: background-color 0.2s;
}

.switch-pref-row:hover {
  background-color: var(--md-sys-color-surface-container-high);
}

.dropdown-text {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.dropdown-title {
  font-size: 14.5px;
  font-weight: 500;
  color: var(--md-sys-color-on-surface);
}

.pref-inner-divider {
  height: 1px;
  background-color: var(--md-sys-color-outline-variant);
  margin: 0 16px;
  opacity: 0.4;
}

.dialogs-wrapper {
  position: absolute;
  width: 0;
  height: 0;
  overflow: visible;
  pointer-events: none;
  z-index: 500;
}

.dialogs-wrapper :deep(md-dialog) {
  pointer-events: auto;
}
</style>
