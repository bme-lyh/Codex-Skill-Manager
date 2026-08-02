import { describe, expect, it } from "vitest";
import {
  DEFAULT_NAVIGATION_GROUP_ID,
  getDefaultSectionTabId,
  getNavigationGroup,
  getSectionTab,
  getSectionTabs,
  NAVIGATION_GROUPS
} from "./navigation";

describe("shell navigation", () => {
  it("defines the five stable top-level groups", () => {
    expect(DEFAULT_NAVIGATION_GROUP_ID).toBe("home");
    expect(NAVIGATION_GROUPS.map(group => group.id)).toEqual([
      "home",
      "assets",
      "security",
      "activity",
      "settings"
    ]);
    expect(NAVIGATION_GROUPS.every(group => group.label.zhCN && group.label.enUS && group.icon)).toBe(true);
    expect(getNavigationGroup("security").badgeKey).toBe("securityRiskCount");
  });

  it("defines asset and activity tabs with a deterministic default", () => {
    expect(getSectionTabs("assets").map(tab => tab.id)).toEqual(["skills", "groups"]);
    expect(getDefaultSectionTabId("assets")).toBe("skills");
    expect(getSectionTabs("activity").map(tab => tab.id)).toEqual([
      "updates",
      "history",
      "quarantine",
      "reports"
    ]);
    expect(getDefaultSectionTabId("activity")).toBe("updates");
  });

  it("falls back safely for unknown groups and tab ids", () => {
    expect(getNavigationGroup("unknown").id).toBe("home");
    expect(getDefaultSectionTabId("unknown")).toBeUndefined();
    expect(getSectionTab("assets", "unknown")?.id).toBe("skills");
    expect(getSectionTab("security", "unknown")).toBeUndefined();
  });
});
