# Skill discovery

A candidate is a directory containing `SKILL.md`. Discovery walks a staged
GitHub archive or explicit local package, skips `.git`, `.system`, dependency
and build-output directories, and rejects symbolic links.

The candidate name comes from the directory basename. `SKILL.md` frontmatter is
parsed for display metadata but does not override the install directory name.
One repository may contain many independent candidates; all retain the same
source package relationship in the lock and GUI graph.
