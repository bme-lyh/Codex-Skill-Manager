import { createContext, useContext, useEffect, useMemo } from "react";
import type { PropsWithChildren } from "react";

export type AppLocale = "zh-CN" | "en-US";
export type Translate = (zhCN: string, enUS: string) => string;

/** Stable shell copy. Feature-specific details can continue to use `t`, but
 * navigation and safety-critical terms are kept in one auditable place. */
export const uiCopy = {
  home: ["首页", "Home"],
  assets: ["资产", "Assets"],
  skills: ["Skills", "Skills"],
  groups: ["分组", "Groups"],
  security: ["安全", "Security"],
  activity: ["活动", "Activity"],
  updates: ["更新", "Updates"],
  history: ["历史与回滚", "History & Rollback"],
  quarantine: ["隔离区", "Quarantine"],
  reports: ["报告", "Reports"],
  settings: ["设置", "Settings"],
  checkUpdates: ["检查更新", "Check updates"],
  codexReview: ["Codex 复核", "Codex review"],
  sourceGroup: ["来源分组", "Source group"]
} as const satisfies Record<string, readonly [string, string]>;

export function copy(key: keyof typeof uiCopy, locale: AppLocale): string {
  const [zhCN, enUS] = uiCopy[key];
  return locale === "en-US" ? enUS : zhCN;
}

type I18nValue = {
  locale: AppLocale;
  t: Translate;
  formatDate: (value: string | number | Date) => string;
  join: (values: string[]) => string;
};

const I18nContext = createContext<I18nValue | null>(null);

export function normalizeLocale(value: unknown): AppLocale {
  return value === "en-US" ? "en-US" : "zh-CN";
}

export function I18nProvider({ locale, children }: PropsWithChildren<{ locale: AppLocale }>) {
  useEffect(() => {
    document.documentElement.lang = locale;
  }, [locale]);

  const value = useMemo<I18nValue>(() => ({
    locale,
    t: (zhCN, enUS) => locale === "en-US" ? enUS : zhCN,
    formatDate: value => new Date(value).toLocaleString(locale),
    join: values => values.join(locale === "en-US" ? ", " : "、")
  }), [locale]);

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

export function useI18n(): I18nValue {
  const value = useContext(I18nContext);
  if (!value) throw new Error("useI18n must be used inside I18nProvider");
  return value;
}

export function translate(locale: AppLocale, zhCN: string, enUS: string): string {
  return locale === "en-US" ? enUS : zhCN;
}
