<script setup lang="ts">
/**
 * @file AboutScreen.vue
 * @description 关于页：展示 Logo / 应用名 / 版本号，并提供「查看源码」「加入 Telegram」外链入口。
 *   版本号从 KernelSU 模块信息读取，外链经系统 Intent 打开。子页形态（带返回顶栏）。
 */
import { ref, onMounted } from 'vue';
import { useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { getModuleInfo, openExternalUrl } from '../utils/ksu';

const router = useRouter();
const { t } = useI18n();
const appVersion = ref('v8.0.0-alpha.2');

/** 返回上一页（子页顶栏返回箭头）。 */
const handleBack = () => {
  router.back();
};

/**
 * 用系统默认应用（浏览器 / Telegram）打开外部链接。
 * @param url  目标链接
 */
const openLink = (url: string) => {
  openExternalUrl(url);
};

/** 使用管理器提供的模块元数据刷新版本号。 */
const loadVersion = () => {
  const info = getModuleInfo();
  if (typeof info?.version === 'string' && info.version.trim()) {
    appVersion.value = info.version.trim();
  }
};

onMounted(() => {
  loadVersion();
});
</script>

<template>
  <Teleport to="body">
    <div class="sub-screen-overlay scroll-container">
      <!-- 顶栏：返回 + 标题 -->
      <header class="sub-top-bar">
        <div class="sub-top-bar-left">
          <md-icon-button @click="handleBack" class="sub-back-btn">
            <md-icon>
              <svg viewBox="0 0 24 24">
                <path d="M20 11H7.83l5.59-5.59L12 4l-8 8 8 8 1.41-1.41L7.83 13H20v-2z" fill="currentColor"/>
              </svg>
            </md-icon>
          </md-icon-button>
          <h1 class="sub-screen-title">{{ t('settings.about') }}</h1>
        </div>
      </header>

      <div class="sub-screen-content">
        <div class="about-page-container">
          <!-- 头部：Logo / 应用名 / 版本号 -->
          <div class="about-header-section">
            <div class="about-logo-box">
              <img src="/favicon.svg" class="about-logo-img" alt="NetProxy logo" />
            </div>
            <h1 class="about-app-name">NetProxy</h1>
            <span class="about-app-version">{{ appVersion }}</span>
          </div>

          <!-- 外链入口：查看源码 / 加入 Telegram -->
          <div class="config-card">
            <div class="arrow-pref-row" @click="openLink('https://github.com/Fanju6/NetProxy-Magisk')">
              <div class="pref-text">
                <span class="pref-title">{{ t('about.viewSource') }}</span>
              </div>
              <div class="pref-arrow-icon">
                <svg viewBox="0 0 24 24">
                  <path d="M10 6L8.59 7.41 13.17 12l-4.58 4.59L10 18l6-6-6-6z" fill="currentColor"/>
                </svg>
              </div>
            </div>

            <div class="pref-inner-divider"></div>

            <div class="arrow-pref-row" @click="openLink('https://t.me/NetProxy_Magisk')">
              <div class="pref-text">
                <span class="pref-title">{{ t('about.joinTelegram') }}</span>
              </div>
              <div class="pref-arrow-icon">
                <svg viewBox="0 0 24 24">
                  <path d="M10 6L8.59 7.41 13.17 12l-4.58 4.59L10 18l6-6-6-6z" fill="currentColor"/>
                </svg>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
/* 子页覆盖层与顶栏（与原 SettingsScreen 一致的层叠样式） */
.sub-screen-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
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
  from {
    opacity: 0;
    transform: translateY(30px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.sub-top-bar {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: calc(64px + var(--top-inset));
  padding-top: var(--top-inset);
  padding-left: 16px;
  padding-right: 16px;
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
  padding: 16px;
  padding-bottom: calc(24px + var(--bottom-inset));
  box-sizing: border-box;
  width: 100%;
}

.config-card {
  background-color: var(--md-sys-color-surface-container);
  border-radius: var(--radius-xl);
  border: none;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  width: 100%;
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
  user-select: none;
}

.arrow-pref-row:hover {
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

.pref-inner-divider {
  display: none;
}

/* 关于页专属布局：头部 Logo / 应用名 / 版本号 */
.about-page-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  width: 100%;
  max-width: 800px;
  margin: 0 auto;
  gap: 24px;
}

.about-header-section {
  display: flex;
  flex-direction: column;
  align-items: center;
  margin-top: 40px;
  margin-bottom: 12px;
}

.about-logo-box {
  width: 88px;
  height: 88px;
  border-radius: 24px;
  background-color: var(--md-sys-color-surface-container);
  box-shadow: var(--md-sys-elevation-1);
  overflow: hidden;
  display: flex;
  align-items: center;
  justify-content: center;
}

.about-logo-img {
  width: 100%;
  height: 100%;
  object-fit: contain;
}

.about-app-name {
  font-size: 35px;
  font-weight: bold;
  color: var(--md-sys-color-on-background);
  margin: 12px 0 4px 0;
}

.about-app-version {
  font-size: 14px;
  color: var(--md-sys-color-on-surface-variant);
  margin-bottom: 24px;
}
</style>
