# Troubleshooting

## The desktop window does not open

- Confirm that Windows 10/11 has the WebView2 Runtime.
- Run `csm doctor`.
- Check write access to the configured data directory.
- Keep the visible error text and the newest file under `data\logs` when
  reporting the problem.

## A private repository returns 401 or 404

- Run `gh auth status`.
- Confirm the token can read repository contents.
- Remember that GitHub may return 404 when a private repository exists but the
  current credential cannot access it.

## The install dialog returns GitHub 403

Open **Settings → GitHub credentials and limits** and verify the credential.
If the app shows a reset time, wait before retrying. The install dialog keeps
your source input and analysis state. A downloaded local folder can be used
while GitHub is unavailable.

## Codex enhanced analysis is unavailable

- In **Settings**, check the Codex CLI path, sign-in status, and available models.
- Confirm that the account has available usage.
- For a private source, confirm that Codex CLI may process its contents.
- An MCP plan needs a real Git or SVN project directory, not staging or a link.
- Retry once, or keep the completed local assessment and use standard Skill installation.

## Installation or rollback stops

Read the transaction timeline and recovery status in the install dialog or
**History**. A failed or cancelled operation first restores completed reversible
steps in reverse order. If recovery is incomplete, use the recorded rollback
action and keep the transaction ID. Do not move or delete backup paths by hand.

If another program changed a managed file after installation, automatic
rollback refuses to overwrite it and shows a manual recovery path instead.

## A Skill cannot update

- Check for local edits.
- Check the repository, path, and ref recorded in the source lock.
- Create a fresh plan instead of reusing an expired plan ID.

For a recorded failed transaction, run:

```powershell
csm rollback --transaction <ID>
```
