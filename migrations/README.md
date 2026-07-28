# migrations/

Hand-written DDL for the **hiring** database, versioned for review and audit.

**There is no migration runner.** The production schema is managed externally — apply
these files manually (DBA / Cloud SQL console) **before** deploying the code that
depends on them, and before bumping the `hire-sdk` version in the API repos. Files are
named `YYYYMMDD_<ticket>_<summary>.sql`, with `<ticket>` omitted when there isn't one,
and must be idempotent (`IF NOT EXISTS` / `IF EXISTS`) so re-running is safe.

The hiring database is **shared by apen, nurse and phar** through `chat.app_id`; there
is one physical database, so each file is applied once and takes effect for all three
apps. This is why the DDL lives here rather than being copied into the three API repos:
their `migrations/` directories cover their own main databases only.

Deployment order for a schema change that this SDK depends on:

1. Apply the SQL to the hiring database
2. Merge and tag `hire-sdk`
3. Bump `go.mod` in `apen-api`, `nurse-api` and `phar-api` — **all three together**,
   or older code keeps running older logic against the same database

| File | Ticket | Applies to |
|---|---|---|
| `20260728_add_chat_thread_hidden_cleared.sql` | — | hiring DB |
