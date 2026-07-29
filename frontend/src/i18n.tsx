import { createContext, useContext, useEffect, useMemo } from "react";
import type { PropsWithChildren } from "react";

export type AppLocale = "zh-CN" | "en-US";
export type Translate = (zhCN: string, enUS: string) => string;

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
