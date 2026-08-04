# Agent entry point

Codex Skill Manager (CSM) is a local-first Windows application for discovering,
installing, updating, auditing, quarantining and restoring Codex skills.

Agents should prefer `csm --json` and treat every mutating operation as a
preview-and-apply workflow:

1. prepare or inspect the intended source group;
2. apply it only after the user has approved the complete group and any group
   risk decision.

Version 0.14.0 manages both `.codex/skills` and `.agents/skills`. Install and
update are source-group operations; Skill records remain child diagnostics.
Every Skill, group, scan, update, and transaction is identified by `rootId`; omitted install
targets default to `codex-default` only for compatibility. New integrations
must send the root explicitly. Both `.system` directories are read-only.

The app uses one **Source → Understand & plan → Check & confirm → Install & result**
workflow for every GitHub and local source. The application identifies the
project type and installation route automatically, runs mandatory layered checks,
and reports one of four gates: installable, needs confirmation, blocked, or
incomplete. Codex review is available from the first install screen; one human
confirmation binds the reviewed source, report, permissions, and plan. Persist and inspect the source-bound preview, assessment, locally
derived typed steps and permissions, explicit project root, progress sequence,
parent transaction, and recovery status. Codex is an optional semantic check
provider, not a separate installation mode; its output is untrusted proposal
data and must never become shell authority. Unsupported work remains manual.

Never edit `sources.lock.json` or `state.db` directly. Never delete skill
directories. `remove` means a reversible move into quarantine. The `.system`
directory is protected and cannot be managed.

Start with:

```powershell
csm doctor --json
csm dashboard --json
csm audit --json
```

Read [command-contracts.md](command-contracts.md),
[transaction-model.md](transaction-model.md), and
[security-policy.md](security-policy.md) before automating mutations.
