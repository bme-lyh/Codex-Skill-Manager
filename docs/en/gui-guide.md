# Desktop GUI guide

The unified desktop shell has five primary areas: **Home**, **Assets**,
**Security**, **Activity**, and **Settings**. Assets and Activity expose their
own secondary tabs. The **Add project** dialog uses one four-step flow for every
source; it always starts with required local checks, then exposes Codex review
on the first screen. One human confirmation is enough to run the digest-bound
reviewed plan. Critical findings remain a safety boundary; High findings use a
single explicit confirmation without a typed reason.

In 0.15.0, the source group is the operation unit. One GitHub repository maps to
one group, and every valid Skill in that group is installed or updated together.
Group security reports support one-click human approval for every severity;
immutable commits, paths, hashes, complete snapshots, and recovery checks still
apply. Skill rows remain details for diagnosis and recovery, not independent
management targets.

## Navigation

Use the root selector at the top of the sidebar to switch between **Codex
Skills** and **Agents Skills**. Lists, groups, security, updates, history, and
quarantine follow the selected root. The sidebar collapses, while appearance can
follow Windows or be set explicitly to Light or Dark; high contrast remains
available through Windows.

- **Home** shows managed, unmanaged, and system Skill counts, open reports,
  source groups, and recent operations.
- **Assets** contains **Skills** and **Groups**.
- **Security** contains the local Skill scanner and risk-review results.
- **Activity** contains **Updates**, **History & Rollback**, **Quarantine**, and
  **Reports**.
- **Settings** contains language, appearance, storage, GitHub, scheduled checks,
  optional Codex review, and diagnostics.

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
   or an absolute local directory containing one or more Skills, and asks which
   Skills root should receive the result. GitHub input is
   resolved to a full commit. A local directory is copied to a bounded,
   manager-owned snapshot before review.
2. **Understand & plan** reads project documentation and installation markers,
   classifies the project, discovers Codex Skills, inventories the source, and
   scans the actual write targets. Required, conditionally triggered, and
   optional checks are shown separately. This local phase does not call Codex,
   download dependencies, or execute repository scripts.
3. **Check & confirm** shows the evidence, project classification, the complete
   source group, exact targets, findings, permissions, and reversibility. Its four
   conclusions are **Ready to install**, **Review before installing**,
   **Installation blocked**, and **Assessment incomplete**. Only the first two
   can proceed, and the latter two fail closed.
4. **Install & result** applies the complete source group or approved structured
   steps, shows progress and activity, records a parent transaction, and refreshes the
   latest Skills and operation status after the change. The result keeps the
   transaction ID, completed targets, manual work, and recovery state visible.

The target root is part of the plan. Once review begins, applying to another
root is rejected; return to Source and analyze again. Same-name Skills may live
in both roots, but bulk actions never cross roots.

Adding a project starts one unified flow. **Codex review and controlled install**
is available on the first screen. It reuses the local assessment, sends bounded
source context with shell access disabled, and returns a project overview,
security conclusion, evidence limitations, and declarative installation methods.
After review, one human confirmation creates the digest-bound plan and starts
the existing journaled executor.

Codex text is never executed as a command. Automatic steps are limited to
complete source-group Skill installation, an exact-version Python tool from official PyPI,
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
risk result, and the source group remain available. **Standard group install**
writes only the complete source-group Skill directories and does not configure MCP or extra
dependencies.

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

Critical, High, Medium, and Low findings can be approved with one human action
for the whole source group. The app records that decision without asking for a
typed reason. Immutable refs, path containment, snapshot completeness, hashes,
expiry, and recovery checks remain mandatory; changing the source or report
invalidates the approval.

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
and allows restoration. Each report opens its findings, risk clusters, ignored
state, and Codex conclusions in place.

## Settings

Language supports Simplified Chinese and English. Appearance supports System,
Light, and Dark. Chinese and System are used on first run and when no saved
settings exist. Switching language or appearance is immediate, saved
automatically, and restored on the next launch.

Configure absolute paths, scheduled read-only checks, private GitHub
credentials, optional Security-page Codex review, and diagnostics. Credentials
are stored in Windows Credential Manager and are not written to logs or reports.
The Codex status area checks the configured CLI, sign-in state, capabilities, and
visible model catalog. Codex requires an installed, signed-in CLI and consumes
account usage.
