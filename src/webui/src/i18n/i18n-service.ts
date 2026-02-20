import zhCN from "./zh-CN.js";
import enUS from "./en-US.js";
import urPK from "./ur-PK.js";

/** Language dictionary type - maps translation keys to translated strings */
export type LanguageDictionary = Record<string, string>;

/** Supported language codes */
export type SupportedLanguage = "zh-CN" | "en-US" | "ur-PK";

/** All available language resources */
export type LanguageResources = Record<SupportedLanguage, LanguageDictionary>;

/** Translation parameters for string interpolation */
export type TranslationParams = Record<string, string | number>;

/**
 * I18nService - Internationalization Service
 * Handles language switching, translation lookup, and persistence.
 */
export class I18nService {
  static STORAGE_KEY: string = "language";
  static DEFAULT_LANG: SupportedLanguage = "zh-CN"; // Default fallback
  static currentLang: SupportedLanguage = "zh-CN";

  // Translation Resources
  static resources: LanguageResources = {
    "zh-CN": zhCN as LanguageDictionary,
    "en-US": enUS as LanguageDictionary,
    "ur-PK": urPK as LanguageDictionary,
  };

  // Initialize
  static init(): void {
    const savedLang = localStorage.getItem(this.STORAGE_KEY);
    if (savedLang && this.resources[savedLang as SupportedLanguage]) {
      this.currentLang = savedLang as SupportedLanguage;
    } else {
      // Auto detect
      const navLang = navigator.language;
      if (navLang.startsWith("en")) {
        this.currentLang = "en-US";
      } else {
        this.currentLang = "zh-CN"; // Default to Chinese
      }
    }
    document.documentElement.lang = this.currentLang;
    this.applyLanguage();
  }

  // Set Language
  static setLanguage(lang: string): void {
    if (lang === "auto") {
      localStorage.removeItem(this.STORAGE_KEY);
      this.init(); // Re-detect
    } else if (this.resources[lang as SupportedLanguage]) {
      this.currentLang = lang as SupportedLanguage;
      localStorage.setItem(this.STORAGE_KEY, lang);
      document.documentElement.lang = lang;
      this.applyLanguage();
    }
  }

  static getLanguage(): string {
    return localStorage.getItem(this.STORAGE_KEY) || "auto";
  }

  // Translate
  static t(key: string, params: TranslationParams = {}): string {
    const dict =
      this.resources[this.currentLang] || this.resources[this.DEFAULT_LANG];
    let text = dict[key] || key;

    // Replace params: {name} -> value
    for (const [k, v] of Object.entries(params)) {
      text = text.replace(new RegExp(`\\{${k}\\}`, "g"), String(v));
    }
    return text;
  }

  // Apply translation to all [data-i18n] elements
  static applyLanguage(): void {
    // Set directionality
    if (this.currentLang === "ur-PK") {
      document.documentElement.dir = "rtl";
    } else {
      document.documentElement.dir = "ltr";
    }

    const selectors = [
      "[data-i18n]",
      "[data-i18n-placeholder]",
      "[data-i18n-label]",
      "[data-i18n-headline]",
      "[data-i18n-helper]",
      "[data-i18n-description]",
    ];
    const elements = document.querySelectorAll(selectors.join(","));

    elements.forEach((el) => {
      const key = el.getAttribute("data-i18n");
      if (key) {
        el.textContent = this.t(key);
      }

      const attrs = [
        "placeholder",
        "label",
        "headline",
        "helper",
        "description",
      ];
      attrs.forEach((attr) => {
        const k = el.getAttribute(`data-i18n-${attr}`);
        if (k) el.setAttribute(attr, this.t(k));
      });
    });

    // Dispatch event for components to update themselves
    window.dispatchEvent(
      new CustomEvent("language-changed", {
        detail: { lang: this.currentLang },
      }),
    );
  }
}
