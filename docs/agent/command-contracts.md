# Command contracts

Global options are accepted before the command:

```text
csm --config <absolute-config-path> --json <command>
```

Read-only commands:

- `doctor`, `dashboard`/`discover`, `audit [--skill NAME]`, `check`;
- `history`, `reports`, `version`.

`audit` without `--skill` explicitly scans every non-system Skill and records
application group metadata. With `--skill`, it uses the same selective scan path
for that named Skill.

`check` accepts repeatable `--group` and optional `--force`, then persists and
returns structured per-source status (`up-to-date`, `update-available`,
`unsupported`, `rate-limited`, or `error`) plus timestamps, remote Commit,
per-Skill current Commits, and explicit outdated Skill names.

`github-auth` validates credential/quota state. `codex status` is read-only and
returns authentication plus capability-based compatibility without a version pin;
after authentication it also returns the visible model catalog reported by the
current CLI, with catalog failures kept separate from compatibility failures.
`codex review --report ID [--skill NAME ...]` creates a journaled advisory
review. Repeated `--skill` values restrict work to trusted Skills discovered
inside the persisted report target. Omitting the flag reviews all discovered
Skills. The result is stable per Skill. All selected Skills in the same application
group share one review task; only different groups execute concurrently.
If at least one group succeeds, the command returns the partial report; its
review and journal status are `partial`, with failed groups named explicitly.

Planning commands:

- `update --group ID`: resolve a GitHub source and create a persisted update
  preview for its installed Skills; scan scope is limited to actual candidate
  Skill directories;
- `install --url URL [--ref REF]`: create a preview only;
- `install --local PATH`: create a preview only.

Mutating commands:

- `bootstrap`: manage the two known existing packages without replacing files;
- `manage --skill NAME...`: create a persisted source-detection and scan plan;
- `manage --plan-id ID --apply --skill NAME...`: apply an explicit plan;
- `group create --name NAME`;
- `group rename --id ID --name NAME`;
- `group reorder --id ID...`;
- `group move --group ID --skill NAME...`;
- `install --plan-id ID --apply --skill NAME... [--accept-high-risk]`;
- `remove NAME...`: move explicit names into quarantine;
- `restore --skill NAME --transaction ID`;
- `rollback --transaction ID`;
- `warning --fingerprint HASH --rule ID --file PATH [--reason TEXT]`:
  persist an ignore decision; `--restore` removes it;
- `warning --cluster ID --fingerprint HASH...`: apply one human decision to a
  complete cluster;
- `warning --report SCAN_ID [--dry-run] [--restore]`: preview or atomically
  apply one human decision to every matching cluster in a report;
- `schedule`: create or update a scheduled check.

Names must be explicit and cannot contain wildcards. A plan expires after 24
hours. Active High/Critical findings block apply until ignored by an explicit
human action. Reasons are optional. `--accept-high-risk`,
`--deterministic`, and `--confirm-deterministic` remain accepted for backward
compatibility but do not create a separate decision path. Callers must preserve
the returned transaction ID.

Managing existing Skills is metadata-only: it snapshots `sources.lock.json`,
detects sources, hashes the current files and records a `manage` transaction.
Legacy `adopt` remains an alias. Group mutations snapshot `groups.json` and
never mutate source provenance or Skill content.
Finding/cluster ignores are reloaded from local state at apply time. Ignoring
creates a transaction record and the reason is optional; restored findings immediately
participate in the gate again. A Codex verdict can never substitute for the
explicit deterministic confirmation.
