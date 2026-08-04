export type AppTheme = "system" | "light" | "dark";

export function normalizeTheme(value: unknown): AppTheme {
  return value === "light" || value === "dark" ? value : "system";
}

export function applyTheme(theme: AppTheme): void {
  const root = document.documentElement;
  if (theme === "system") {
    root.removeAttribute("data-theme");
    root.style.removeProperty("color-scheme");
    return;
  }
  root.dataset.theme = theme;
  root.style.colorScheme = theme;
}
