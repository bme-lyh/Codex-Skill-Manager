import { describe, expect, it, vi } from "vitest";
import { applyTheme, normalizeTheme } from "./theme";

describe("appearance settings", () => {
  it("accepts only the three persisted theme modes", () => {
    expect(normalizeTheme("system")).toBe("system");
    expect(normalizeTheme("light")).toBe("light");
    expect(normalizeTheme("dark")).toBe("dark");
    expect(normalizeTheme("unknown")).toBe("system");
    expect(normalizeTheme(undefined)).toBe("system");
  });

  it("keeps explicit native control chrome in sync with the selected mode", () => {
    const previousDocument = (globalThis as { document?: unknown }).document;
    const style = {
      colorScheme: "",
      removeProperty: vi.fn((property: string) => {
        if (property === "color-scheme") style.colorScheme = "";
      })
    };
    const root: {
      dataset: Record<string, string>;
      style: typeof style;
      removeAttribute: (name: string) => void;
    } = {
      dataset: {},
      style,
      removeAttribute(name) {
        if (name === "data-theme") delete this.dataset.theme;
      }
    };
    (globalThis as { document?: unknown }).document = { documentElement: root };

    try {
      applyTheme("light");
      expect(root.dataset.theme).toBe("light");
      expect(style.colorScheme).toBe("light");

      applyTheme("dark");
      expect(root.dataset.theme).toBe("dark");
      expect(style.colorScheme).toBe("dark");

      applyTheme("system");
      expect(root.dataset.theme).toBeUndefined();
      expect(style.colorScheme).toBe("");
      expect(style.removeProperty).toHaveBeenCalledWith("color-scheme");
    } finally {
      if (previousDocument === undefined) {
        delete (globalThis as { document?: unknown }).document;
      } else {
        (globalThis as { document?: unknown }).document = previousDocument;
      }
    }
  });
});
