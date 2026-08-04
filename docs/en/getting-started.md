# Getting started

## Run a release build

1. Download and extract the Windows release archive.
2. Keep both `CodexSkillManager.exe` and `csm.exe` in the same directory.
3. Double-click `CodexSkillManager.exe`.
4. The app lists both Codex and Agents roots; new installations default to Codex.

Portable releases contain `portable.marker` and store configuration and runtime
state under the release directory. Standard installations store application
state under `%USERPROFILE%\.codex\skill-manager`.

The first-run interface is Simplified Chinese. Open **Settings → Language** and
choose **English**. On first run, those labels appear as **设置 → 语言**. The
change is immediate and saved automatically.

The shell has five primary areas: **Home**, **Assets**, **Security**,
**Activity**, and **Settings**. Assets contains **Skills** and **Groups**;
Activity contains **Updates**, **History & Rollback**, **Quarantine**, and
**Reports**.

## Add a project

Open **Add project** from the top-right action. The dialog uses the same four
steps for a GitHub link and a local directory:

1. **Source**: enter a repository, directory, or direct `SKILL.md` link. A
   GitHub source is pinned to a full commit. A local source is copied into a
   bounded, manager-owned snapshot.
2. **Understand & plan**: the app reads project documentation and markers,
   classifies the project, discovers Codex Skills, and scans the exact targets.
   Required, conditional, and optional checks are shown separately. The local
   phase does not call Codex, download dependencies, or execute repository
   scripts.
3. **Check & confirm**: review evidence, the complete source group, exact write targets,
   risks, permissions, and recovery. The four conclusions are **Ready to
   install**, **Review before installing**, **Installation blocked**, and
   **Assessment incomplete**. Only the first two can continue.
4. **Install & result**: approve the complete source group or structured steps. The
   dialog shows progress, records a parent transaction, refreshes the latest Skills and
   operation status, and keeps the recovery state visible.

The first screen now offers **Codex review and controlled install**. After the
mandatory local assessment, Codex performs a bounded read-only review. One human
confirmation binds the source, report, permissions, and plan before the existing
journaled installer runs. **Standard Skill install** remains an explicit
alternative; the manager never executes repository scripts or dependency,
publishing, and cleanup commands.

The assessment groups checks automatically and ends in one of four UI outcomes.
Expired, replaced, digest-mismatched, unknown, unsupported, or case-variant
`.system` targets fail closed and require a fresh check. `.system` is always
read-only. A project with no Codex Skill is not copied to the global Skills
folder, and an MCP project directory must be selected explicitly.

The app reads both `%USERPROFILE%\.codex\skills` and
`%USERPROFILE%\.agents\skills`. Choose the target before analysis; the plan
locks that root before apply. Same-name Skills remain independent, and both
`.system` directories are read-only. After installation,
the dialog refreshes the Skills list and operation status. If the refresh fails,
the completed result is preserved; retry the refresh instead of repeating the
installation. Use **History & Rollback** for the transaction and rollback path,
or **Quarantine** to restore removed Skills.

## Add the bundled Codex Skill

Release archives include `agent-skill\codex-skill-manager`. To add it to the
global Codex Skills directory, open **Add project**, choose **Local directory**,
select the archive's `agent-skill` directory, review the four steps, and install
the complete source group. A source checkout provides the same content at
`skills\codex-skill-manager`.

## Manage existing Skills

Open **Assets → Skills**, select unmanaged items, and choose **Manage**. The
read-only preview shows detected repositories, source paths, proposed groups,
confidence, and scan results. Confirming the plan records provenance and hashes;
it does not move or rewrite Skill content.

## Codex review and controlled installation

The Codex review action is for repositories that also need a
Python tool or Codex MCP integration. Check Codex in **Settings** and make sure
the CLI is installed, signed in, and has available usage. The app packages the
prepared source with shell access disabled and returns a project summary,
security conclusion, evidence limits, declarative methods, typed steps, and
permissions.

Automatic execution is limited to complete source-group Skill installation, a verified
repository-matched Python tool from official PyPI, and a manager-owned Codex MCP
entry. Repository scripts, free-form commands, unsupported work, and other
unsafe-to-automate steps remain manual. Managed Python and MCP automation
requires a GitHub source so package ownership can be verified; MCP needs a real
Git or SVN project directory. A partial result records the completed automatic
steps and the remaining manual work.

The read-only scan does not download dependencies. Only after you approve plan
generation may the app download the complete Wheel closure from official PyPI
into isolated staging and lock its filenames and SHA256 values. No package is
installed or run during analysis. Source distributions are rejected and
native-code Wheels require separate High-risk approval. Source context is sent
through Codex CLI, so private-repository users should consider their data and
usage requirements before enabling the optional scan.

Source, GitHub 403, Codex, and execution errors stay visible inside the Add
project dialog. Rate limits show a reset countdown. You can retry or cancel;
long-running work can continue after hiding the dialog, and reopening restores
its progress. After a failed run, the approval view restores only the exact
prior Skill subset, permissions, and project root for a new review. Failure or
cancellation recovers reversible steps in reverse order.

The CLI retains its lower-level compatibility contract for scripted workflows;
see the [CLI reference](cli-reference.md#codex-assisted-installation).

## Check for updates

Updates are source-group operations in 0.14.0. A GitHub repository is one
group; the preview always includes every valid member and the apply action
cannot submit a subset. Internal Skill failures continue independently and the
parent group transaction records completed, partial, failed, and recovery
states.

Open **Activity → Updates** and click **Check updates**. This compares commits
only. Select one or more available sources, review the immutable commit, exact
targets, findings, and Skill selection, then apply the chosen updates. Each
source has an independent transaction, backup, and rollback entry.

No scheduled check installs updates automatically.
