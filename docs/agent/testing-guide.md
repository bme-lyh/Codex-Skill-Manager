# Testing guide

Use an isolated absolute `skillsRoot`; never point mutation tests at the live
Codex directory.

```powershell
go test ./...
go vet ./...
cd frontend
pnpm run build
cd ..
go run github.com/wailsapp/wails/v2/cmd/wails@v2.13.0 build -s -m -trimpath
```

End-to-end coverage should exercise local preview/apply, conflict handling,
single and batch warning decisions, multi-skill rollback, quarantine/restore,
update checks and report creation. GitHub tests should use a public fixture for
preview only and must not depend on the repository default branch remaining
unchanged.

After the versioned binaries pass validation, create the standard archive,
portable archive, and SHA-256 manifest with:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass `
  -File .\scripts\package-release.ps1 -Version 0.7.7
```

The packager refuses to overwrite an existing output directory. Release assets
are written under `build/release/<version>` and are not committed. The bundled
Agent Skill must appear exactly once at
`agent-skill/codex-skill-manager/SKILL.md`.
