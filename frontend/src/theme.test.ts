import { describe, expect, it } from "vitest";
import { normalizeTheme } from "./theme";

describe("appearance settings", () => {
  it("accepts only the three persisted theme modes", () => {
    expect(normalizeTheme("system")).toBe("system");
    expect(normalizeTheme("light")).toBe("light");
    expect(normalizeTheme("dark")).toBe("dark");
    expect(normalizeTheme("unknown")).toBe("system");
    expect(normalizeTheme(undefined)).toBe("system");
  });
});
