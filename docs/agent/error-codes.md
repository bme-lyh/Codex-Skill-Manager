# Error and exit conventions

Version 1 uses typed command context plus stable message prefixes rather than a
separate numeric domain-code registry.

- exit `0`: command completed;
- exit `1`: operational, validation, policy or transaction failure;
- exit `2`: missing command or invalid CLI usage.

Automation should branch on JSON `status`, then inspect the command-specific
error. Never infer success from an empty stdout stream. Future domain codes may
be added without removing the existing fields.
