# Private GitHub repositories

Codex Skill Manager looks for GitHub credentials in this order:

1. `GITHUB_TOKEN` or `GH_TOKEN` in the process environment.
2. An authenticated GitHub CLI session (`gh auth token`).
3. Windows Credential Manager.

Use a short-lived, fine-grained token with only the repository permissions you
need. Never paste a token into a Skill, log, repository URL, or command
argument.

Open **Settings → GitHub credentials and limits** to verify the current
credential and view the REST API limit and reset time. Credentials are also
used for public repositories, which avoids the lower shared-IP limit. If GitHub
returns a rate-limit error, the app keeps the last successful state, shows the
reset countdown, and lets you retry after credentials are fixed.
