# Codex Skill Manager

> **Review the source. See the risks. Install with a way back.**

Codex Skill Manager is a local Windows app for inspecting, installing, updating,
grouping, and recovering Codex Skills. It keeps every write explicit,
journaled, and recoverable.

[Download v0.10.0](https://github.com/bme-lyh/Codex-Skill-Manager/releases/tag/v0.10.0) ·
[Get started](docs/en/getting-started.md) ·
[User guide](docs/en/gui-guide.md) ·
[Security](SECURITY_EN.md) ·
[中文](README_ZH.md)

![Windows 10/11](https://img.shields.io/badge/Windows-10%20%2F%2011-2563eb)
![Version](https://img.shields.io/badge/version-0.10.0-187a69)
![Go](https://img.shields.io/badge/Go-1.26-00add8)
![Wails](https://img.shields.io/badge/Wails-v2-cc1f45)
![License](https://img.shields.io/badge/license-MIT-334155)
[![CI](https://github.com/bme-lyh/Codex-Skill-Manager/actions/workflows/ci.yml/badge.svg)](https://github.com/bme-lyh/Codex-Skill-Manager/actions/workflows/ci.yml)

## Interface

[![Codex Skill Manager interface preview](docs/images/ui-carousel.gif)](docs/images/ui-carousel.gif)

The preview uses fictional Skills, groups, and paths. It contains no real
account or personal data.

## What it does

| Task | Result |
|---|---|
| Install | Load a public/private GitHub source or a local folder, then install only the selected Skills |
| Assess | Read project documentation, discover Skills, scan exact targets, and show required, triggered, and optional checks |
| Update | Compare source commits, review the exact changes, and back up existing content before replacement |
| Recover | Restore from backups, quarantine, or a recorded rollback transaction |
| Organize | Manage existing Skills without moving them; create and reorder groups |
| Automate | Use the bundled `csm` CLI and Codex Skill with structured JSON output |

## One workflow, clear choices

Every GitHub or local source follows the same flow:

1. **Source** — pin a GitHub commit or create a bounded local snapshot.
2. **Assess** — read project instructions, identify Codex Skills, scan the real
   targets, and stop on unsupported or changed input.
3. **Review** — check the selected files, risks, permissions, and recovery path.
4. **Apply** — write only the approved targets and record the transaction.

For a normal Skill repository, standard installation copies only the selected
Skill directories. Optional **Codex enhanced analysis** can explain a complex
project and propose supported Python-tool or MCP steps. It requires an
installed, signed-in Codex CLI and consumes Codex usage. It never replaces the
required local assessment.

If Codex Skills are found, selected Skills go to
`%USERPROFILE%\.codex\skills` by default. `.system` is always read-only. A
project with no Codex Skill is not silently installed into that global folder;
unsupported work stays manual, and any supported MCP project directory must be
chosen explicitly.

## Download and run

1. Download the standard or portable Windows archive from
   [GitHub Releases](https://github.com/bme-lyh/Codex-Skill-Manager/releases/latest).
2. Extract the archive to a stable directory.
3. Run `CodexSkillManager.exe`.

Portable builds store data beside the app. Standard builds use
`%USERPROFILE%\.codex\skill-manager`. The binaries are not Windows code-signed,
so SmartScreen may show a warning. Download only from this repository and
verify the archive against the included checksum file:

```powershell
Get-FileHash .\CodexSkillManager-0.10.0-windows-amd64.zip -Algorithm SHA256
```

Compare the result with `CodexSkillManager-0.10.0-SHA256SUMS.txt`.

### Install the bundled Codex Skill

Release archives include `agent-skill\codex-skill-manager`. To add it to the
global Codex Skills directory without copying files by hand, open **Install
Skill**, choose **Local folder**, select the archive's `agent-skill` directory,
review the assessment, and apply the selected Skill. From a source checkout,
use `skills\codex-skill-manager` instead.

## Safety boundaries

- Repository scripts and free-form commands are never executed.
- High risk requires individual confirmation and a reason; Critical risk
  cannot be ignored.
- Report-wide ignore applies only to known Medium-or-lower findings.
- Replacements are backed up, removals go to quarantine, and rollback refuses
  to overwrite content changed after installation.
- Optional Codex analysis is read-only until you separately approve a typed
  plan and every required permission.

A clean scan is not proof that a Skill is safe. Read the
[security policy](SECURITY_EN.md) before approving unfamiliar sources.

## Build from source

Install Go, Node.js, pnpm, WebView2, and Wails v2, then run:

```powershell
pnpm --dir frontend install --frozen-lockfile
.\scripts\build.ps1
```

The desktop app and CLI are written to `build\bin`. The GUI must be built
through Wails; plain `go build` omits required desktop tags.

## Documentation

- [English documentation](docs/en/README.md)
- [中文文档](docs/README.md)
- [CLI reference](docs/en/cli-reference.md)
- [Architecture and Agent development](docs/agent/AGENT-ENTRYPOINT.md)
- [Contributing](CONTRIBUTING.md)

Codex Skill Manager 0.10.0 supports Windows 10/11 and is licensed under the
[MIT License](LICENSE).
