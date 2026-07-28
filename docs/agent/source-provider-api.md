# Source provider API

A source provider must return:

- provider ID and canonical source identity;
- requested and resolved refs;
- immutable revision identifier;
- default branch;
- local staging root;
- candidate skill names and source-relative paths.

Provider implementations must stage into `stagingRoot`, enforce bounded input,
reject links and traversal, and never write directly to `skillsRoot`.
Version 1 ships `github` and `local`. GitHub accepts repository, `/tree/...`,
and `/blob/.../SKILL.md` URLs for public or authenticated private repositories.
