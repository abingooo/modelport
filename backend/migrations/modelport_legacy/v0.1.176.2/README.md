# ModelPort legacy migration archive

This directory preserves the exact SQL blobs shipped by
`custom-v0.1.176.2` (`origin/production` at `b6cb4d0c8`). It is intentionally
outside the active top-level `*.sql` migration glob and must never be executed
by the v0.1.183+ runner.

`manifest.tsv` records both the raw blob SHA-256 and the checksum used by the
legacy runner (`SHA-256(strings.TrimSpace(content))`). The archive exists to
verify production `schema_migrations` rows and to support bridge tests without
renumbering, editing, or re-running historical SQL. `223_group_model_pricing.sql`
is included because ModelPort renamed that upstream migration to avoid its old
custom `221` collision.

Columns in `manifest.tsv` are:

1. historical filename
2. SHA-256 of the exact archived bytes
3. legacy runner checksum

Never edit an archived SQL file. Add a new active migration instead.
