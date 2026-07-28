# Relationship model

The relationship graph is:

```text
provider -> source package -> skill -> installed file hashes
```

A source package is keyed by provider and canonical repository or local path.
Source groups are deterministic and remain the authority for update and
security operations. A separate local layout layer allows source-group labels,
manual groups, ordering, and per-Skill assignments. Moving a Skill in that
layout never changes its source package. System groups remain read-only.
