# Source provider API

A source provider must return:

- provider ID and canonical source identity;
- requested and resolved refs;
- immutable revision identifier for remote sources, or a revalidated content
  digest for an explicit local source;
- default branch;
- prepared source root;
- candidate skill names and source-relative paths.

Remote providers must stage into `stagingRoot`, enforce bounded input, reject
links and traversal, and never write directly to `skillsRoot`. The local
provider accepts an explicit absolute directory and binds the plan to its
inventoried content; apply must revalidate it before copying. Version 1 ships
`github` and `local`. GitHub accepts repository, `/tree/...`, and
`/blob/.../SKILL.md` URLs for public or authenticated private repositories.
