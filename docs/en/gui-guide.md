# Desktop GUI guide

The unified desktop shell has five primary areas: **Home**, **Assets**,
**Security**, **Activity**, and **Settings**. Assets and Activity expose their
own secondary tabs. The **Add project** dialog uses one four-step flow for every
source; it always starts with required local checks before any optional Codex
work. Review exact targets, permissions, and recovery information before
writing. Critical findings cannot be ignored; High findings require per-cluster
confirmation and a non-empty reason.

## Navigation

- **Home** shows managed, unmanaged, and system Skill counts, open reports,
  source groups, and recent operations.
- **Assets** contains **Skills** and **Groups**.
- **Security** contains the local Skill scanner and risk-review results.
- **Activity** contains **Updates**, **History & Rollback**, **Quarantine**, and
  **Reports**.
- **Settings** contains language, storage, GitHub, scheduled checks, optional
  Codex review, and diagnostics.

The header's **Add project** action is available from the main shell. Existing
Page routes and the Wails/API compatibility boundary remain for integrations,
but the visible UI uses the labels in this guide.

## Home

Home is a status overview, not a second installation surface. It summarizes
managed and unmanaged Skills, read-only system Skills, open security reports,
groups, and recent installation, update, management, and rollback activity.

## Assets

### Skills

Search and inspect source, status, version, and local-change information. Select
all, invert, or clear the current results. Selected unmanaged Skills can be
analyzed and managed; selected personal Skills can be moved to **Quarantine**.
System Skills are not selectable.

Managing an existing Skill creates a read-only provenance and safety plan.
Confirming it records source information and hashes; it does not move or rewrite
Skill content. Source groups are detected from trusted directories, explicit
`SKILL.md` provenance, Git remotes, and GitHub links. An uncertain remote source
is kept in its own local group.

### Groups

Source groups remain the authority for updates. Display groups can be created,
renamed, reordered, and populated by dragging Skills. Layout changes are
journaled and reversible; they do not change the Skill's source content.

## Add project

The dialog has four visible steps:

1. **Source** accepts a GitHub repository, directory, or direct `SKILL.md` link,
   or an absolute local directory containing one or more Skills. GitHub input is
   resolved to a full commit. A local directory is copied to a bounded,
   manager-owned snapshot before review.
2. **Understand & plan** reads project documentation and installation markers,
   classifies the project, discovers Codex Skills, inventories the source, and
   scans the actual write targets. Required, conditionally triggered, and
   optional checks are shown separately. This local phase does not call Codex,
   download dependencies, or execute repository scripts.
3. **Check & confirm** shows the evidence, project classification, Skill
   selection, exact targets, findings, permissions, and reversibility. Its four
   conclusions are **Ready to install**, **Review before installing**,
   **Installation blocked**, and **Assessment incomplete**. Only the first two
   can proceed, and the latter two fail closed.
4. **Install & result** applies the selected Skills or approved structured
   steps, shows progress and activity, records a transaction, and refreshes the
   latest Skills and operation status after the change. The result keeps the
   transaction ID, completed targets, manual work, and recovery state visible.

Adding a project starts one unified flow. For a complex repository, **More options** contains **Run enhanced
project scan**. This is an explicit, optional semantic scan provided by Codex,
not a separate installation mode. It reuses the local assessment, sends bounded
source context with shell access disabled, and returns a project overview,
security conclusion, evidence limitations, and declarative installation methods.
The scan does not authorize installation, create a plan, or download
dependencies. Review it and choose **Approve and create installation plan**
before typed steps and permissions can be proposed.

Codex text is never executed as a command. Automatic steps are limited to
selected Skill installation, an exact-version Python tool from official PyPI,
and a manager-owned Codex MCP entry. Repository scripts, arbitrary shell
commands, unsupported package managers, and other work remain manual. Managed
Python and MCP automation requires a GitHub source so package ownership can be
verified; an MCP target must be a real, explicitly selected Git or SVN project
directory and an existing same-name entry is not replaced.

If a Python tool is approved in a generated plan, dependency resolution obtains
the complete Wheel closure from official PyPI into isolated staging and locks
filenames and SHA256 values. Nothing is installed or run during analysis.
Source distributions are rejected; native-code Wheels require separate High-risk
approval. Apply verifies the locked files and installs offline.

Every required permission must be approved before execution. The dialog can be
hidden while a long task continues in the background; reopening restores its
latest progress. If a Codex plan cannot be generated, the source assessment,
risk result, and Skill selection remain available. **More options** also offers
**Switch to standard installation**, which writes only the selected Skill
directories and does not configure MCP or extra dependencies.

After failure or cancellation, reversible steps are recovered in reverse order.
A partial result keeps unsupported manual steps and its rollback entry. If the
operation completes but the dashboard refresh fails, the result is preserved;
retry the refresh instead of running installation again. Rollback refuses to
overwrite a target changed after installation and provides a manual recovery
path. Use **History & Rollback** for transaction recovery and **Quarantine** for
removed Skills.

## Security

Skills without a trusted scan record or with changed content are selected by
default. Results follow **Group → Skill → Warning**, with details collapsed by
default for large reports. Repeated scans count unique active High/Critical
findings from the latest effective report per target.

Critical findings always block writes and cannot be ignored. High findings need
per-cluster confirmation and a non-empty reason; batch actions cannot bypass
that rule. Medium and lower findings use the ordinary ignore flow. Restoring a
previously accepted High cluster immediately reactivates the gate.

The Security page's optional **Codex review** packages the selected group with
shell access disabled. It is advisory, treats local rule hits as supplemental
leads, and leaves the final decision to the user. The Settings toggle controls
this Security review only; it does not silently enable the Add project scan.

## Activity

### Updates

Updates compares source commits and reports a clear status and last-check time
for each source. Review the immutable commit, exact target files, security
results, and Skill selection before applying. Each source is an independent
transaction with its own backup and rollback entry; one failure does not hide
completed sources. Scheduled checks are read-only and never install updates
automatically.

### History & Rollback

History lists mutations, transaction status, progress, and recovery information.
When a backup exists, it offers rollback. Recovery stops safely rather than
overwriting a managed target that changed after installation.

### Quarantine and Reports

Quarantine stores explicitly removed personal Skills without permanent deletion
and allows restoration. Reports provide human-readable and structured audit
records for local scans and operations.

## Settings

Language supports Simplified Chinese and English. Chinese is used on first run
and when no saved language exists. Switching language is immediate, saved
automatically, and restored on the next launch.

Configure absolute paths, scheduled read-only checks, private GitHub
credentials, optional Security-page Codex review, and diagnostics. Credentials
are stored in Windows Credential Manager and are not written to logs or reports.
The Codex status area checks the configured CLI, sign-in state, capabilities, and
visible model catalog. Codex requires an installed, signed-in CLI and consumes
account usage.
