# Scanner provider API

A scanner receives a bounded local directory and produces a stable report ID,
timestamp, highest severity and ordered findings. Each finding contains rule
ID, severity, file, line, summary and evidence suitable for review.

Scanner providers must not mutate content. Rule IDs and severities are public
compatibility surfaces. A provider failure blocks installation rather than
silently returning a clean report. External providers must declare data sent
off-device and require explicit opt-in.

The built-in scanner inventories paths synchronously so file-count, symlink and
size boundaries remain deterministic, then scans eligible text files with a
bounded worker pool. Providers may parallelize read-only content inspection but
must return a stable order independent of worker completion order.
