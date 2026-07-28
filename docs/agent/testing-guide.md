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
high-risk approval, multi-skill rollback, quarantine/restore, update checks and
report creation. GitHub tests should use a public fixture for preview only and
must not depend on the repository default branch remaining unchanged.
