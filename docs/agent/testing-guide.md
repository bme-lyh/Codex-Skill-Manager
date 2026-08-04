# Testing guide

Use an isolated absolute `skillsRoot`; never point mutation tests at the live
Codex directory.

```powershell
cd frontend
pnpm install --frozen-lockfile
pnpm run test
pnpm run build
cd ..
go test ./...
go vet ./...
go run ./cmd/skill-validator ./skills/codex-skill-manager
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\build.ps1
```

End-to-end coverage should exercise local preview/apply, conflict handling,
single and batch warning decisions, multi-skill rollback, quarantine/restore,
update checks and report creation. GitHub tests should use a public fixture for
preview only and must not depend on the repository default branch remaining
unchanged.

Assisted-install tests must also cover source and context-digest binding,
full-context packaged Codex invocation with shell disabled, schema/finalizer rejection of
free-form commands, permission omission, plan/configuration drift, exact
Skill selection, PyPI repository matching, Wheel limits and archive traversal,
proxy allowlisting and cancellation, sanitized subprocess environments,
MCP project-root and ownership conflicts,
monotonic progress, cancellation, reverse recovery, and recovery hash drift.
Fixtures must not execute scripts from a repository or mutate a real Codex
configuration. CLI tests must cover both `install --assist` phases, repeated
`--grant`, `--all`, conditional `--project-root`, JSON error data, and the
independence of assisted opt-in from the Security Center review toggle.

For a release, first commit the intended source and confirm the worktree is
clean. Then run `scripts/build.ps1` again from that exact clean `HEAD`; do not
edit, generate tracked files, or amend the commit after the build. Finally
create the standard archive, portable archive, standalone build manifest, and
unsigned-release notice plus SHA-256 manifest with:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass `
  -File .\scripts\package-release.ps1 -Version 0.14.0
```

The build script records the source commit, dirty flag, version, and binary
hashes plus toolchain versions in `build/bin/build-manifest.json`. The packager
requires that manifest to match the clean release `HEAD` and both binaries, and
refuses to overwrite an existing output directory. Release assets are written under
`build/release/<version>` and are not committed. The bundled Agent Skill must
appear exactly once at
`agent-skill/codex-skill-manager/SKILL.md`.
