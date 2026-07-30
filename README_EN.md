# Codex Skill Manager

> **Understand the source and the risk before a Skill reaches Codex.**

Codex Skill Manager is a local Windows 10/11 application for downloading, scanning, installing, updating, uninstalling, and grouping Codex Skills. Every change is designed to be traceable and recoverable.

[Latest release](https://github.com/bme-lyh/Codex-Skill-Manager/releases/latest) · [Getting started](docs/en/getting-started.md) · [GUI guide](docs/en/gui-guide.md) · [Security](SECURITY_EN.md) · [中文](README.md)

## Interface preview

[![Current Codex Skill Manager interface preview: grouped Skills, batch actions, update status, Security Center, Codex review, GitHub and local installation, Codex assisted plans and results, rollback, reports, and Chinese/English settings](docs/images/ui-carousel.gif)](docs/images/ui-carousel.gif)

> The animation uses fictional Skills, groups, and paths. It contains no real account or personal information.

## Purpose

Skills change how Codex works. Manual copying is easy, but later it may be hard to tell where a Skill came from, whether it changed, or how to recover it.

This app installs and organizes Skills in one place, checks common risks, and keeps recovery data before updates or removal.

## Core features

| Feature | What it does |
|---|---|
| Install | Install one or more Skills from public/private GitHub repositories or local folders, with optional Codex assistance for complex repositories |
| Risk checks | Show common risks and related files before installation, updates, or manual review |
| Update and remove | Support multi-selection, automatic backups, quarantine, and recovery |
| Groups | Group automatically or create, rename, and drag your own groups |
| Existing Skills | Add current Skills to management without moving their files |
| Interface language | Switch between Simplified Chinese and English; Chinese is the first-run default |

## Two installation modes

Both modes first confirm the source, find the Skills, and run the local risk
check.

| Mode | Best for | Behavior |
|---|---|---|
| Standard installation | Ordinary single- or multi-Skill repositories | Installs only the selected Skill directories; it does not install tools or change MCP configuration |
| Codex assisted installation | Complex repositories that also need tools or MCP setup | Codex reads the complete repository and creates an explanation and plan; after you approve the Skills, permissions, and project directory, the app runs only supported steps |

Assisted installation requires an installed and signed-in **Codex CLI** and
consumes Codex usage. The app does not directly run repository scripts or
temporary commands written by Codex. Anything that cannot be completed safely
is clearly left as a manual step. When reviewing a Python tool, the app may
download files from official PyPI into isolated staging for verification; it
does not install or run them before approval.

Progress, errors, remaining manual steps, and rollback actions stay available
in the installation dialog or History. Standard installation remains available
when Codex is unavailable. See the [GUI guide](docs/en/gui-guide.md) and
[security policy](SECURITY_EN.md) for the detailed boundaries.

When a repository contains different same-name mainline and Codex variants,
the dialog lists the conflicting paths and can switch to a detected Codex
subtree. A failed Codex plan does not discard the completed source check, so
the selected Skills can still be installed in standard mode.

## Optional Codex review

Codex review is off by default. To use it, install and sign in to **Codex CLI**, enable the feature in Settings, and make sure your Codex account has available usage.

Reviews consume Codex usage. Time and usage depend on the number and size of Skills, the selected model, and reasoning effort. The app reviews one group at a time, retries a failed group once, and keeps progress and results when you switch pages. All local checks and management features still work without Codex review.
This Settings toggle controls Security Center review only; it does not control an explicitly selected Codex assisted installation.

## Quick start

### Release build

1. Download the Windows archive from **GitHub Releases**.
2. Extract it to a stable directory.
3. Run `CodexSkillManager.exe`.
4. Open **Skills**, select unmanaged items, and choose **Manage**.

The default Skills root is `%USERPROFILE%\.codex\skills`. The `.system` directory is always read-only.
The interface starts in Simplified Chinese. Open **设置 → 语言**, then choose **English**; the change is immediate and saved automatically.
Current release archives are not Windows code-signed, so SmartScreen may show
a warning. Download only from this repository's Releases page and compare the
archive hash with the accompanying `SHA256SUMS.txt`:

```powershell
Get-FileHash .\CodexSkillManager-0.9.0-windows-amd64.zip -Algorithm SHA256
```

### Build from source

Install Go, Node.js, pnpm, WebView2, and Wails v2, then run:

```powershell
pnpm --dir frontend install --frozen-lockfile
.\scripts\build.ps1
```

Outputs are written to `build\bin`. The GUI must be built through Wails; plain `go build` omits required desktop build tags.

## Safety and recovery

- Downloaded content is not executed automatically.
- The app shows targets and risks before changing files.
- Updates create backups, and removed Skills go to quarantine for recovery.
- Local changes are not replaced without clear approval.
- Codex review is optional; the default checks stay local.
- Codex assisted installation runs only supported, locally validated, explicitly approved steps.

A clean static scan is not proof of safety. See the [security policy](SECURITY_EN.md) for limitations and vulnerability reporting.

## Documentation

- [English documentation](docs/en/README.md)
- [Chinese documentation](docs/README.md)
- [CLI reference](docs/en/cli-reference.md)
- [Architecture](docs/agent/architecture.md)
- [Contributing](CONTRIBUTING.md)

Current version: **0.9.0**. Licensed under the [MIT License](LICENSE).
