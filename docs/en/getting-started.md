# Getting started

## Run a release build

1. Download and extract the Windows release archive.
2. Keep both `CodexSkillManager.exe` and `csm.exe` in the same directory.
3. Double-click `CodexSkillManager.exe`.
4. The default Skills root is `%USERPROFILE%\.codex\skills`.

Portable releases contain `portable.marker` and store configuration and runtime state under the release directory. Standard installations store application state under `%USERPROFILE%\.codex\skill-manager`.

## Manage existing Skills

Open **Skills**, select unmanaged items, and click **Manage**. The read-only preview shows detected repositories, source paths, proposed groups, confidence, and scan results. Confirming the plan records provenance and hashes; it does not move or rewrite Skill content.

## Install a new Skill

Click **Install Skill**, choose GitHub or local directory, and provide the source. GitHub input may be a repository URL, a path inside a repository, or a direct `SKILL.md` URL. The application resolves a commit, downloads to staging, discovers Skills, scans actual installation targets, and asks you to select the final targets.

## Check for updates

Open **Update Center** and click **Check updates**. This only compares commits. Select one or many sources, review the generated plans and findings, then select the individual Skills to update.

No scheduled check installs updates automatically.
