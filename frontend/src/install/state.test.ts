import { describe, expect, it } from "vitest";
import type {
  AssistedInstallPlan,
  AssistedInstallProgress,
  AssistedInstallStep
} from "../types";
import {
  ACTIVE_INSTALL_REFERENCE_VERSION,
  assessmentAllowsSelectedTargets,
  assistedPlanDisposition,
  classifyInstallIssue,
  createActiveInstallReference,
  mergeProgressSnapshot,
  parseActiveInstallReference,
  parseRetryTimestamp,
  restoredSelectedSkills,
  retryWaitMilliseconds,
  serializeActiveInstallReference
} from "./state";

describe("layered assessment gate", () => {
  const targets = [
    { kind: "codex-skill" as const, displayName: "safe-skill", path: "skills/safe-skill", supported: true, permissionIds: [], reversible: true },
    { kind: "application" as const, displayName: "app", path: ".", supported: false, permissionIds: [], reversible: false }
  ];

  it("allows only supported Skill targets after a ready or attention assessment", () => {
    expect(assessmentAllowsSelectedTargets({ gate: "ready", targets }, ["safe-skill"])).toBe(true);
    expect(assessmentAllowsSelectedTargets({ gate: "attention", targets }, ["safe-skill"])).toBe(true);
    expect(assessmentAllowsSelectedTargets({ gate: "ready", targets }, ["app"])).toBe(false);
  });

  it("fails closed for missing, blocked, incomplete, or empty selections", () => {
    expect(assessmentAllowsSelectedTargets(null, ["safe-skill"])).toBe(false);
    expect(assessmentAllowsSelectedTargets({ gate: "blocked", targets }, ["safe-skill"])).toBe(false);
    expect(assessmentAllowsSelectedTargets({ gate: "incomplete", targets }, ["safe-skill"])).toBe(false);
    expect(assessmentAllowsSelectedTargets({ gate: "ready", targets }, [])).toBe(false);
  });
});

describe("classifyInstallIssue", () => {
  it("classifies a GitHub rate-limit 403 and preserves its retry time", () => {
    const issue = classifyInstallIssue({
      code: "github_api_error",
      message: "GitHub API 403 Forbidden：请求额度已用尽，可在 2026-07-30 18:15:30 后重试"
    });

    expect(issue.githubForbidden).toBe(true);
    expect(issue.rateLimited).toBe(true);
    expect(issue.retryAt).toBe("2026-07-30 18:15:30");
  });

  it("keeps a repository permission 403 separate from rate limiting", () => {
    const issue = classifyInstallIssue(
      new Error("GitHub API: 403 Forbidden: Resource not accessible by integration")
    );

    expect(issue.githubForbidden).toBe(true);
    expect(issue.rateLimited).toBe(false);
  });

  it("extracts a suggested Codex subtree from a Skill variant conflict", () => {
    const issue = classifyInstallIssue(new Error(
      'multiple different Skills use the name "ablation-planner"; ' +
      "conflicting repository paths: skills/ablation-planner, skills/skills-codex/ablation-planner; " +
      "suggested Codex source URL: https://github.com/owner/repo/tree/abc123/skills/skills-codex"
    ));

    expect(issue.skillVariantConflict).toBe(true);
    expect(issue.suggestedSourceUrl).toBe(
      "https://github.com/owner/repo/tree/abc123/skills/skills-codex"
    );
  });

  it("calculates a bounded retry countdown", () => {
    const retryAt = "2026-07-30T10:15:30Z";
    const timestamp = parseRetryTimestamp(retryAt);

    expect(retryWaitMilliseconds(retryAt, timestamp - 30_000)).toBe(30_000);
    expect(retryWaitMilliseconds(retryAt, timestamp + 1)).toBe(0);
    expect(retryWaitMilliseconds(undefined, timestamp)).toBe(0);
    expect(parseRetryTimestamp("2026-07-30 18:15:30")).toBe(
      new Date(2026, 6, 30, 18, 15, 30).getTime()
    );
  });
});

describe("mergeProgressSnapshot", () => {
  it("ignores duplicate and out-of-order progress from the same run", () => {
    const current = progress({ sequence: 4, message: "newer" });
    const stale = progress({ sequence: 2, message: "stale" });
    const duplicate = progress({ sequence: 4, message: "duplicate" });

    expect(mergeProgressSnapshot(current, stale)).toBe(current);
    expect(mergeProgressSnapshot(current, duplicate)).toBe(current);
  });

  it("accepts a newer event and a different run", () => {
    const current = progress({ sequence: 4, message: "current" });
    const newer = progress({ sequence: 5, message: "newer" });
    const nextRun = progress({ runId: "tx-next", sequence: 1, message: "next run" });

    expect(mergeProgressSnapshot(current, newer)).toBe(newer);
    expect(mergeProgressSnapshot(current, nextRun)).toBe(nextRun);
  });
});

describe("active installation reference migration", () => {
  it("round-trips the versioned typed reference", () => {
    const reference = createActiveInstallReference("plan", "assisted-plan-123");
    const parsed = parseActiveInstallReference(serializeActiveInstallReference(reference));

    expect(parsed).toEqual({ reference, migrated: false });
    expect(reference.version).toBe(ACTIVE_INSTALL_REFERENCE_VERSION);
  });

  it("migrates both legacy plain ids and the unversioned object", () => {
    expect(parseActiveInstallReference("plan-legacy")).toEqual({
      reference: createActiveInstallReference("analysis", "plan-legacy"),
      migrated: true
    });
    expect(parseActiveInstallReference(JSON.stringify({
      kind: "plan",
      id: "assisted-plan-legacy"
    }))).toEqual({
      reference: createActiveInstallReference("plan", "assisted-plan-legacy"),
      migrated: true
    });
  });

  it("rejects malformed or unknown references", () => {
    expect(parseActiveInstallReference('{"kind":"plan"}')).toBeNull();
    expect(parseActiveInstallReference("not-a-plan")).toBeNull();
    expect(parseActiveInstallReference(null)).toBeNull();
  });
});

describe("restored plan state", () => {
  it("defaults a legacy plan without selectedSkills to an empty selection", () => {
    const plan = {} as Pick<AssistedInstallPlan, "selectedSkills">;

    expect(restoredSelectedSkills(plan)).toEqual([]);
  });

  it("restores only the exact persisted selection", () => {
    const plan = {
      selectedSkills: ["alpha", "beta", "alpha"]
    } as Pick<AssistedInstallPlan, "selectedSkills">;

    expect(restoredSelectedSkills(plan)).toEqual(["alpha", "beta"]);
  });
});

describe("assisted plan disposition", () => {
  it("marks a required manual-only plan", () => {
    const disposition = assistedPlanDisposition({
      status: "manual-required",
      steps: [step({ kind: "manual", required: true, supported: false })]
    });

    expect(disposition).toEqual({
      manualRequired: true,
      manualOnly: true,
      partial: true,
      supportedStepCount: 0
    });
  });

  it("marks a mixed plan as partial but still executable", () => {
    const disposition = assistedPlanDisposition({
      status: "manual-required",
      steps: [
        step({ id: "automatic", supported: true }),
        step({ id: "manual", kind: "manual", required: true, supported: false })
      ]
    });

    expect(disposition.manualOnly).toBe(false);
    expect(disposition.partial).toBe(true);
    expect(disposition.supportedStepCount).toBe(1);
  });

  it("keeps a fully automatic plan complete", () => {
    const disposition = assistedPlanDisposition({
      status: "ready",
      steps: [step({ supported: true })]
    });

    expect(disposition.manualRequired).toBe(false);
    expect(disposition.manualOnly).toBe(false);
    expect(disposition.partial).toBe(false);
  });
});

function progress(
  overrides: Partial<AssistedInstallProgress> = {}
): AssistedInstallProgress {
  return {
    referenceId: "assisted-plan-123",
    runId: "tx-current",
    sequence: 1,
    phase: "running",
    message: "running",
    completedSteps: 0,
    totalSteps: 1,
    activityCount: 1,
    steps: [],
    startedAt: "2026-07-30T10:00:00Z",
    updatedAt: "2026-07-30T10:00:01Z",
    terminal: false,
    ...overrides
  };
}

function step(overrides: Partial<AssistedInstallStep> = {}): AssistedInstallStep {
  return {
    id: "step",
    kind: "install-skills",
    title: "Install skills",
    description: "",
    status: "queued",
    required: true,
    supported: false,
    reversible: true,
    ...overrides
  };
}
