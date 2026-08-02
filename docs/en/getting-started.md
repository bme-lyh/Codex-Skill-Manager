# Getting started

## Run a release build

1. Download and extract the Windows release archive.
2. Keep both `CodexSkillManager.exe` and `csm.exe` in the same directory.
3. Double-click `CodexSkillManager.exe`.
4. The default Skills root is `%USERPROFILE%\.codex\skills`.

Portable releases contain `portable.marker` and store configuration and runtime state under the release directory. Standard installations store application state under `%USERPROFILE%\.codex\skill-manager`.

The first-run interface is Simplified Chinese. Open **Settings → Language** and
choose **English**. On first run, those labels appear as **设置 → 语言**. The
change is immediate and saved automatically.

The release also includes `agent-skill\codex-skill-manager`. To install this
helper into the global Codex Skills directory, open **Install Skill**, choose
**Local folder**, select the release's `agent-skill` directory, review the
assessment, and apply the Skill.

## Install a source build to a chosen directory

From the repository root, build the application and then run the source-only
installation script:

```powershell
pnpm --dir frontend install --frozen-lockfile
.\scripts\build.ps1
.\scripts\install.ps1 -InstallDirectory "D:\Apps\CodexSkillManager"
```

If Windows execution policy blocks the source installation script, use:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass `
  -File .\scripts\install.ps1 `
  -InstallDirectory "D:\Apps\CodexSkillManager"
```

Release archives do not require this script: extract the selected archive and
run the executable directly.

## Manage existing Skills

Open **Skills**, select unmanaged items, and click **Manage**. The read-only preview shows detected repositories, source paths, proposed groups, confidence, and scan results. Confirming the plan records provenance and hashes; it does not move or rewrite Skill content.

## Install a new Skill

Click **Install Skill**, choose GitHub or local directory, and provide the
source. GitHub input may be a repository URL, a path inside a repository, or a
direct `SKILL.md` URL. The dialog uses one **Source → Assess → Review → Apply**
workflow. GitHub input is pinned to an immutable commit; an explicit local
directory is copied into a bounded manager-owned snapshot. The mandatory local
assessment reads documentation and installation markers, classifies the project,
discovers Codex Skills, scans actual targets, and groups checks as required,
triggered, or optional. Only `ready` and `attention` assessments can continue.
Expired, replaced, digest-mismatched, unknown, or case-variant `.system` targets
fail closed and require a fresh assessment.

Discovered Skills are installed under the configured Skills root, which defaults
to `%USERPROFILE%\.codex\skills`. A project with no Codex Skill is not copied
there as an ordinary application. Unsupported work remains manual; an MCP
project directory, when needed, must be chosen explicitly.

**Standard installation** copies only the selected Skill directories. It does
not install extra tools, configure MCP, or execute repository scripts.

**Optional Codex enhanced analysis** is for repositories that also need a Python
tool or Codex MCP integration. It never replaces the mandatory local assessment.
Clicking **Run enhanced project scan** is the explicit opt-in for the current
installation. Check Codex in Settings and make sure the CLI is
installed, signed in, and has available usage. The app packages the complete
prepared source for Codex with shell access disabled, then shows a summary, requirements,
typed steps, and required permissions. Select the Skills, approve each required
permission, and provide a real Git or SVN project directory if MCP configuration
needs one.

Automatic execution is limited to installing Skills, installing a verified
Python tool from official PyPI, and writing a manager-owned Codex MCP entry.
Repository scripts and free-form commands proposed by Codex are never run.
Unsupported work remains a manual step. The app can finish supported steps
first, then reports a partial result until the manual work is complete. Managed
Python and MCP automation requires a GitHub source so PyPI ownership can be
verified. The read-only enhanced project scan never downloads dependencies.
Only after you separately approve plan generation may the app download Wheels
from official PyPI into isolated staging to create a complete dependency lock; no
package is installed or run at that stage. Local directories still support
standard installation and packaged Codex analysis. Source context is processed
through Codex CLI, so private-repository users should consider their data and
usage requirements before enabling it.

Source, GitHub 403, Codex, and execution errors stay visible inside the
installation dialog. Rate limits show a reset countdown, and Codex CLI errors
link to Settings. You can retry, cancel, roll back, or preserve the source
analysis and switch to standard installation. Long-running work can continue
after the dialog is hidden. Reopening restores its progress. A failed retry
returns to approval with only the exact prior Skill subset, permissions, and
project root. Partial manual work and its rollback entry remain in History.

The CLI exposes the same layered source, assessment, review, and apply workflow;
see the [CLI reference](cli-reference.md#codex-assisted-installation).

## Check for updates

Open **Update Center** and click **Check updates**. This only compares commits. Select one or many sources, review the generated plans and findings, then select the individual Skills to update.

No scheduled check installs updates automatically.
