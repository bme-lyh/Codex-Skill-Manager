# Security policy

Skills are untrusted instruction packages that may contain executable scripts, prompt injection, credential access, exfiltration instructions, destructive commands, hidden content, or supply-chain substitutions.

Codex Skill Manager pins GitHub refs, protects archive extraction, scans actual
installation targets, never executes repository scripts, blocks active
High/Critical findings by default, backs up replacements, quarantines removals,
protects `.codex/skills/.system`, redacts credentials, and remains local-first.
Known Medium-or-lower findings may be ignored individually or for a whole
report. High requires per-cluster confirmation and a non-empty audit record.
The GUI supplies that record automatically after the one-click human decision;
the CLI continues to require `--reason`. Critical cannot be ignored. Every exact
decision target remains journaled.

Codex risk review and planned installation are explicit opt-in features. They
require a signed-in Codex CLI and consume account usage. Planned installation
packages every prepared source file: text is included verbatim, while binary
files are represented by path, size, and SHA-256. Only root-level `.git`, `.hg`,
or `.svn` directories confirmed as real VCS metadata are skipped. Ordinary
same-name directories and `.csm-backups` or `.csm-quarantine` cannot hide
content from review. Oversized sources are reviewed in bounded no-tools chunks
before a final synthesis. Path containment, file identity, and post-read
checks reject symlink escape or replacement during packaging. Its shell tool
is disabled and its working directory contains only manager-owned output
files. For an explicit local source, do not select a broad parent directory
containing unrelated backups.
Risk review is advisory and cannot create a human ignore decision.
The Settings toggle controls Security Center review only; choosing assisted
installation in the GUI or using CLI `--assist` is the opt-in for that
installation.

Planned installation treats Codex output only as a declarative proposal. Local
validation restricts execution to selected Skill installation, an exact-version
Wheel tool whose official PyPI metadata matches the GitHub repository, and a
manager-owned Codex MCP entry. Repository scripts, arbitrary shell, source
builds, and model-selected paths are never executed. Unsupported work remains
manual; supported steps may finish first, and the result stays partial until
the manual work is complete.

If analysis identifies a Python tool, the app downloads the complete Wheel-only
dependency closure from official PyPI into isolated staging before approval.
The plan binds project, version, filename, compatibility tags, and SHA-256.
Source distributions are rejected. Native Wheels derive a separate
`managed-native-code` high-risk permission. Apply is offline and accepts only
the approved hashes. App-launched pip is forced through a temporary local proxy
that permits only `pypi.org:443` and `files.pythonhosted.org:443`; inherited
proxy and `PIP_*` network settings are removed. TLS remains end-to-end in pip,
and every file must match the filename, URL, and SHA-256 in official PyPI
metadata.

This is application-level egress control for a normal Python/pip process, not
an operating-system network sandbox. A locally compromised Python or pip could
still open its own connection, so planned installation requires a trusted
local Python environment.

Every automatic step requires explicit permission and is journaled. Failure or
cancellation recovers completed reversible steps in reverse order. Recovery
refuses to overwrite a target changed after installation and reports a manual
recovery path instead. Codex MCP configuration is rechecked after the
transaction checkpoint and immediately before atomic replacement; drift stops
the write rather than overwriting the newer content.

A clean static scan is not proof of safety. Dynamic behavior, novel prompt attacks, and runtime remote content may evade deterministic analysis.

Do not disclose real tokens, cookies, private repository content, or immediately exploitable unpatched details in a public issue. Use GitHub private vulnerability reporting when available, or open a minimal non-sensitive contact issue.

Include the affected version and component, reproduction steps using non-sensitive data, expected and actual behavior, and impact. Security fixes target the latest release.
