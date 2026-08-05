<script setup lang="ts">
/**
 * @file SettingsMain.vue
 * @description 设置主页：代理与分应用入口、开机自启、GMS 修复、语言切换、
 *   日志与关于入口。开关状态与切换动作由父级 SettingsLayout 经 provide 注入，本组件只做展示与转发。
 */
import { inject, computed } from 'vue';
import type { Ref } from 'vue';
import { useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import type { SettingsState } from './SettingsLayout.vue';
import { SUPPORTED_LOCALES, setLocale, type LocaleCode } from '../i18n';

const router = useRouter();
const { t, locale, messages } = useI18n();

// 由 SettingsLayout 注入：设置状态 + 各开关的切换动作
const settingsState = inject<Ref<SettingsState>>('settingsState')!;
const toggleAutoStart = inject<() => Promise<void>>('toggleAutoStart')!;
const toggleGmsFix = inject<() => Promise<void>>('toggleGmsFix')!;

/** 跳转到指定子页路由。 */
const navigateTo = (path: string) => {
  router.push(path);
};

// 语言选项：display 取自各 locale 的 lang.display（始终以该语言自身命名显示）
const localeOptions = computed(() =>
  SUPPORTED_LOCALES.map(code => ({
    code,
    display: (messages.value[code] as any)?.lang?.display ?? code
  }))
);

/**
 * 语言下拉变更回调：切换并持久化界面语言。
 * @param e  select 的 change 事件
 */
const onLocaleChange = (e: Event) => {
  setLocale((e.target as HTMLSelectElement).value as LocaleCode);
};
</script>

<template>
  <div class="settings-lazy-column">
    
    <!-- 一、代理设置入口 -->
    <div class="config-card">
      <div class="arrow-pref-row" @click="navigateTo('/settings/proxy')">
        <div class="pref-icon-container">
          <svg viewBox="0 0 24 24">
            <path d="M19 13H5c-.55 0-1 .45-1 1v6c0 .55.45 1 1 1h14c.55 0 1-.45 1-1v-6c0-.55-.45-1-1-1zm-1 6H6v-4h12v4zM19 3H5c-.55 0-1 .45-1 1v6c0 .55.45 1 1 1h14c.55 0 1-.45 1-1V4c0-.55-.45-1-1-1zm-1 6H6V5h12v4z" fill="currentColor"/>
          </svg>
        </div>
        <div class="pref-text">
          <span class="pref-title">{{ t('settings.proxySettings') }}</span>
          <span class="pref-summary">{{ t('settings.proxySettingsDesc') }}</span>
        </div>
        <div class="pref-arrow-icon">
          <svg viewBox="0 0 24 24">
            <path d="M10 6L8.59 7.41 13.17 12l-4.58 4.59L10 18l6-6-6-6z" fill="currentColor"/>
          </svg>
        </div>
      </div>
      <div class="pref-inner-divider"></div>
      <div class="arrow-pref-row" @click="navigateTo('/settings/apps')">
        <div class="pref-icon-container">
          <svg viewBox="0 0 24 24"><path d="M4 8h4V4H4v4zm6 12h4v-4h-4v4zm-6 0h4v-4H4v4zm0-6h4v-4H4v4zm6 0h4v-4h-4v4zm6-10v4h4V4h-4zm-6 4h4V4h-4v4zm6 6h4v-4h-4v4zm0 6h4v-4h-4v4z" fill="currentColor" /></svg>
        </div>
        <div class="pref-text">
          <span class="pref-title">{{ t('settings.apps') }}</span>
          <span class="pref-summary">{{ t('settings.appsDesc') }}</span>
        </div>
        <div class="pref-arrow-icon"><svg viewBox="0 0 24 24"><path d="M10 6 8.59 7.41 13.17 12l-4.58 4.59L10 18l6-6z" fill="currentColor" /></svg></div>
      </div>
    </div>

    <!-- 二、开关项：开机自启 / GMS 修复 -->
    <div class="config-card">
      <!-- 开机自启 -->
      <div class="switch-pref-row" @click="toggleAutoStart">
        <div class="pref-icon-container">
          <svg viewBox="0 0 24 24">
            <path d="M13 3h-2v10h2V3zm4.83 2.17l-1.42 1.42C17.99 7.86 19 9.81 19 12c0 3.87-3.13 7-7 7s-7-3.13-7-7c0-2.19 1.01-4.14 2.58-5.42L6.17 5.17C4.23 6.82 3 9.26 3 12c0 4.97 4.03 9 9 9s9-4.03 9-9c0-2.74-1.23-5.18-3.17-6.83z" fill="currentColor"/>
          </svg>
        </div>
        <div class="pref-text">
          <span class="pref-title">{{ t('settings.autoStart') }}</span>
          <span class="pref-summary">{{ t('settings.autoStartDesc') }}</span>
        </div>
        <md-switch icons :selected="settingsState.autoStartEnabled" @click.stop="toggleAutoStart"></md-switch>
      </div>

      <!-- GMS（Google 服务）修复 -->
      <div class="switch-pref-row" @click="toggleGmsFix">
        <div class="pref-icon-container">
          <svg viewBox="0 0 24 24">
            <path d="M7.5 5.6L10 7L8.6 4.5L10 2L7.5 3.4L5 2L6.4 4.5L5 7L7.5 5.6zm12 9.8L17 14l1.4 2.5L17 19l2.5-1.4L22 19l-1.4-2.5L22 14l-2.5 1.4zM22 2l-2.5 1.4L17 2l1.4 2.5L17 7l2.5-1.4L22 7l-1.4-2.5L22 2zM14.07 8.43L2.69 19.8c-.39.39-.39 1.02 0 1.41.39.39 1.02.39 1.41 0L15.48 9.84l-1.41-1.41z" fill="currentColor"/>
          </svg>
        </div>
        <div class="pref-text">
          <span class="pref-title">{{ t('settings.gmsFix') }}</span>
          <span class="pref-summary">{{ t('settings.gmsFixDesc') }}</span>
        </div>
        <md-switch icons :selected="settingsState.gmsFixEnabled" @click.stop="toggleGmsFix"></md-switch>
      </div>
    </div>

    <!-- 三、界面语言 -->
    <div class="config-card">
      <div class="dropdown-pref-row">
        <div class="pref-icon-container">
          <svg viewBox="0 0 24 24">
            <path d="M12.87 15.07l-2.54-2.51.03-.03c1.74-1.94 2.98-4.17 3.71-6.53H17V4h-7V2H8v2H1v1.99h11.17C11.5 7.92 10.44 9.75 9 11.35 8.07 10.32 7.3 9.19 6.69 8h-2c.73 1.63 1.73 3.17 2.98 4.56l-5.09 5.02L4 19l5-5 3.11 3.11.76-2.04zM18.5 10h-2L12 22h2l1.12-3h4.75L21 22h2l-4.5-12zm-2.62 7l1.62-4.33L19.12 17h-3.24z" fill="currentColor"/>
          </svg>
        </div>
        <div class="pref-text">
          <span class="pref-title">{{ t('lang.title') }}</span>
          <span class="pref-summary">{{ t('lang.summary') }}</span>
        </div>
        <select :value="locale" @change="onLocaleChange" class="pref-dropdown">
          <option v-for="opt in localeOptions" :key="opt.code" :value="opt.code">{{ opt.display }}</option>
        </select>
      </div>
    </div>

    <!-- 四、运行日志与关于入口 -->
    <div class="config-card">
      <!-- 运行日志 -->
      <div class="arrow-pref-row" @click="navigateTo('/settings/logs')">
        <div class="pref-icon-container">
          <svg viewBox="0 0 24 24">
            <path d="M20 19.59V8l-6-6H6c-1.1 0-1.99.9-1.99 2L4 20c0 1.1.89 2 1.99 2H18c.41 0 .78-.12 1.09-.34L20 19.59zM13 3.5L18.5 9H13V3.5zM6 20V4h5v6h6v10H6z" fill="currentColor"/>
          </svg>
        </div>
        <div class="pref-text">
          <span class="pref-title">{{ t('settings.logs') }}</span>
        </div>
        <div class="pref-arrow-icon">
          <svg viewBox="0 0 24 24">
            <path d="M10 6L8.59 7.41 13.17 12l-4.58 4.59L10 18l6-6-6-6z" fill="currentColor"/>
          </svg>
        </div>
      </div>

      <div class="pref-inner-divider"></div>

      <!-- 关于 -->
      <div class="arrow-pref-row" @click="navigateTo('/settings/about')">
        <div class="pref-icon-container">
          <svg viewBox="0 0 24 24">
            <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm1 15h-2v-6h2v6zm0-8h-2V7h2v2z" fill="currentColor"/>
          </svg>
        </div>
        <div class="pref-text">
          <span class="pref-title">{{ t('settings.about') }}</span>
        </div>
        <div class="pref-arrow-icon">
          <svg viewBox="0 0 24 24">
            <path d="M10 6L8.59 7.41 13.17 12l-4.58 4.59L10 18l6-6-6-6z" fill="currentColor"/>
          </svg>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.settings-lazy-column {
  display: flex;
  flex-direction: column;
  gap: 12px;
  width: 100%;
  max-width: 800px;
  margin: 0 auto;
}

.settings-lazy-column .config-card {
  padding: 0;
  gap: 0;
  overflow: hidden;
  background-color: var(--md-sys-color-surface-container);
  border-radius: var(--radius-xl);
  border: none;
  display: flex;
  flex-direction: column;
}

.pref-icon-container {
  width: 24px;
  height: 24px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--md-sys-color-on-surface-variant);
  flex-shrink: 0;
}

.pref-icon-container svg {
  width: 20px;
  height: 20px;
}

.pref-arrow-icon {
  width: 24px;
  height: 24px;
  color: var(--md-sys-color-outline);
  display: flex;
  align-items: center;
  justify-content: center;
}

.pref-arrow-icon svg {
  width: 20px;
  height: 20px;
}

.arrow-pref-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  width: 100%;
  gap: 14px;
  padding: 14px 16px;
  cursor: pointer;
  box-sizing: border-box;
  transition: background-color 0.2s;
}

.arrow-pref-row:hover {
  background-color: var(--md-sys-color-surface-container-high);
}

.switch-pref-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  width: 100%;
  gap: 14px;
  padding: 14px 16px;
  cursor: pointer;
  box-sizing: border-box;
  transition: background-color 0.2s;
}

.switch-pref-row:hover {
  background-color: var(--md-sys-color-surface-container-high);
}

.dropdown-pref-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  width: 100%;
  gap: 14px;
  padding: 14px 16px;
  box-sizing: border-box;
}

.pref-dropdown {
  background-color: var(--md-sys-color-surface-container-high);
  color: var(--md-sys-color-on-surface);
  border: 1px solid var(--md-sys-color-outline-variant);
  padding: 6px 12px;
  border-radius: var(--radius-sm);
  outline: none;
  font-size: 13.5px;
  font-weight: 500;
  cursor: pointer;
  appearance: none;
  flex-shrink: 0;
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

.pref-inner-divider {
  display: none;
}
</style>
