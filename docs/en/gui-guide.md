# Desktop GUI guide

## Overview

Shows managed, unmanaged, and system Skill counts, active high-risk findings, source groups, and recent transactions.

## Skills

Search and inspect complete source, status, and version information. Hover or focus a cell to see untruncated values. Use select all, invert, or clear. Selected unmanaged Skills can be analyzed and managed; selected personal Skills can be moved to quarantine.

## Groups

Source groups are detected automatically and remain the authority for updates. Display groups may be created, renamed, reordered, and populated by dragging Skills. These changes are journaled and reversible.

## Update Center

The page shows the last check time and a clear status for every source. Select one or many available sources, then review the immutable commit, exact target files, security results, and Skill selection for each source.

Each source is applied as an independent transaction with its own backup and rollback entry. A failure in one source is reported without hiding the result of completed sources.

## Security Center

Findings show rule, severity, file, line, evidence, explanation, and recommendation. Repeated scans do not inflate the badge: it counts unique active High/Critical findings from the latest effective report per target. Individual warnings can be ignored with a reason and restored later.

Active Critical findings block installation and update. Each Critical finding may be cleared only after manual review and a non-empty audit reason; restoring it immediately reactivates the gate.

## History, quarantine, and reports

History lists mutations and offers rollback when a backup exists. Quarantine restores removed Skills without permanent deletion. Reports provide human-readable and structured audit records.

## Settings

Configure absolute paths, scheduled read-only checks, private GitHub
credentials, optional Codex review, and diagnostics. When the CLI path is
blank, the application skips unusable WindowsApps candidates and probes PATH
plus the current user's npm directory for a working independent CLI. It checks
required capabilities instead of pinning a CLI version and refreshes status
when focus returns after browser authentication. Once authenticated, the model
picker loads the current CLI's visible model catalog instead of using a
hard-coded list. Background CLI probes run without flashing console windows. Tokens
are stored in Windows Credential Manager and are never written to logs or
reports.
