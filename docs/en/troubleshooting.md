# Troubleshooting

## The desktop window does not open

- Confirm that Windows 10/11 has the WebView2 Runtime.
- Run `csm doctor`.
- Check write access to the configured data directory.
- Keep the visible error text and the newest file under `data\logs` when
  reporting the problem.

## A private repository returns 401 or 404

- Run `gh auth status`.
- Confirm that the credential can read repository contents.
- GitHub may return 404 when a private repository exists but the current
  credential cannot access it.

## Add project returns GitHub 403

Open **Settings → GitHub connection** and verify the current credential. If the
app shows a reset time, wait for the countdown before retrying. **Add project**
keeps the source input and local analysis state. A downloaded local directory
can be used while GitHub is unavailable.

## The project conclusion is blocked or incomplete

The four conclusions are **Ready to install**, **Review before installing**,
**Installation blocked**, and **Assessment incomplete**.

- **Review before installing** means the required checks completed but highlighted
  items need your confirmation.
- **Installation blocked** means a local security policy or unsupported target
  prevents writing; review the evidence or use a different source.
- **Assessment incomplete** means required coverage or evidence is missing; run
  **Check project** again after resolving the source or scan-limit problem.

Only the first two conclusions can continue. A failed local check does not grant
access to the optional Codex scan or dependency planning.

## Enhanced project scan is unavailable

- In **Settings**, check the Codex CLI path, sign-in status, capabilities, and
  visible models.
- Confirm that the account has available usage.
- For a private source, confirm that Codex CLI may process its contents.
- An MCP plan needs a real Git or SVN project directory, not staging or a link.
- Retry **More options → Run enhanced project scan** once, or keep the completed
  local assessment and use **More options → Switch to standard installation**.

Codex is optional. The local assessment and selected Skills remain available
when the semantic scan cannot produce a reliable result.

## Installation completes but the list is stale

The operation result is preserved when the post-install refresh fails. Retry the
refresh shown in the dialog; do not run the installation again. Confirm the
updated Skills and operation status after the refresh.

## Installation or rollback stops

Read the timeline and recovery status in the **Add project** dialog or
**Activity → History & Rollback**. A failed or cancelled operation first
restores completed reversible steps in reverse order. If recovery is incomplete,
use the recorded rollback action and keep the transaction ID. Do not move or
delete backup paths by hand.

If another program changed a managed file after installation, automatic rollback
refuses to overwrite it and shows a manual recovery path instead.

## A Skill cannot update

- Check for local edits.
- Check the repository, path, and ref recorded in the source lock.
- Create a fresh plan instead of reusing an expired plan ID.

For a recorded failed transaction, run:

```powershell
csm rollback --transaction <ID>
```
