import { describe, expect, it } from "vitest";
import { matchesRoot, normalizeRootContract, rootIdentity, rootKindLabel } from "./roots";

describe("root contract", () => {
  it("normalizes v0.11 roots and preserves the explicit default", () => {
    const value = normalizeRootContract({
      defaultRootId: "agents",
      roots: [
        { rootId: "codex-default", rootKind: "codex", rootName: "Codex", path: "C:/codex", enabled: true },
        { rootId: "agents", rootKind: "agents", rootName: "Agents", path: "C:/agents", enabled: true }
      ]
    });
    expect(value.defaultRootId).toBe("agents");
    expect(value.roots.map(root => root.rootId)).toEqual(["codex-default", "agents"]);
  });

  it("accepts the legacy skillRoots spelling and fails closed on an invalid default", () => {
    const value = normalizeRootContract({
      defaultRootId: "missing",
      skillRoots: [{ id: "codex-default", path: "C:/codex", enabled: true }]
    });
    expect(value.defaultRootId).toBe("codex-default");
    expect(value.roots[0].rootKind).toBe("codex");
  });

  it("keeps skill identity root-qualified", () => {
    expect(rootIdentity("agents", "review")).toBe("agents::review");
    expect(rootKindLabel("codex", "zh-CN")).toContain("Codex");
    expect(matchesRoot(undefined, "codex-default")).toBe(true);
    expect(matchesRoot("agents", "codex-default")).toBe(false);
  });
});
