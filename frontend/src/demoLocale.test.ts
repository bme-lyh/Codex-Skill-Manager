import { describe, expect, it } from "vitest";
import {
  demoAssistedInstallPlan,
  demoAssistedInstallProgress,
  demoAssistedInstallResult,
  demoCodexStatus,
  demoConfig,
  demoDashboard,
  demoInstallPreview,
  demoQuarantine,
  demoScanReport
} from "./demo";
import { localizeDemo } from "./demoLocale";

describe("English demo content", () => {
  it("contains no Chinese display text", () => {
    const localized = localizeDemo({
      demoAssistedInstallPlan,
      demoAssistedInstallProgress,
      demoAssistedInstallResult,
      demoCodexStatus,
      demoConfig,
      demoDashboard,
      demoInstallPreview,
      demoQuarantine,
      demoScanReport
    }, "en-US");

    expect(JSON.stringify(localized)).not.toMatch(/[\u3400-\u9fff]/u);
  });
});
