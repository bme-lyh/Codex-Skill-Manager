# Security policy

Skills are untrusted instruction packages that may contain executable scripts, prompt injection, credential access, exfiltration instructions, destructive commands, hidden content, or supply-chain substitutions.

Codex Skill Manager pins GitHub refs, protects archive extraction, scans actual installation targets, never executes repository scripts, blocks active Critical findings until each is manually reviewed and ignored with a recorded reason, requires explicit acceptance for active High findings, backs up replacements, quarantines removals, protects `.codex/skills/.system`, redacts credentials, and remains local-first.

A clean static scan is not proof of safety. Dynamic behavior, novel prompt attacks, and runtime remote content may evade deterministic analysis.

Do not disclose real tokens, cookies, private repository content, or immediately exploitable unpatched details in a public issue. Use GitHub private vulnerability reporting when available, or open a minimal non-sensitive contact issue.

Include the affected version and component, reproduction steps using non-sensitive data, expected and actual behavior, and impact. Security fixes target the latest release.
