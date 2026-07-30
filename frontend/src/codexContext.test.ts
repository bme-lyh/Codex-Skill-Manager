import { describe, expect, it } from "vitest";
import { isPackagedFullContextMode } from "./codexContext";

describe("isPackagedFullContextMode", () => {
  it("recognizes direct and chunk-synthesized complete context", () => {
    expect(isPackagedFullContextMode("full-target-packaged-no-tools")).toBe(true);
    expect(isPackagedFullContextMode("full-target-chunk-summaries-no-tools")).toBe(true);
  });

  it("does not label rule-only or legacy modes as complete packaged context", () => {
    expect(isPackagedFullContextMode("rule-summary")).toBe(false);
    expect(isPackagedFullContextMode("full-target-read-only")).toBe(false);
    expect(isPackagedFullContextMode(undefined)).toBe(false);
  });
});
