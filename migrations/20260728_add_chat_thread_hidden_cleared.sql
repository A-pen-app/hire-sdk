-- Applies to: hiring DB
--
-- Per-user chat room visibility: archive and delete.
--
-- Both columns live on chat_thread, which has one row per participant, so writing one
-- participant's row never changes what the other sees. See docs/chat_visibility.md.
--
--   hidden_at  archive timestamp. The room disappears from this user's list until an
--              activity strictly later than hidden_at occurs. chat.updated_at only
--              moves when a message is added, so a new message restores the room with
--              no extra write on the send path.
--   cleared_at delete cutoff. This user only ever sees messages created strictly after
--              it. Never reset by later messages. Wired up in stage 2; added here so
--              the two visibility columns land together.
--
-- Nullable, no default, no backfill: existing rows read as "never archived, never
-- cleared", which is the behaviour they have today. Idempotent and zero-downtime.
--
-- The hiring DB is shared by apen / nurse / phar via chat.app_id, so this runs once
-- and takes effect for all three apps.
--
-- WARNING: there is no migration runner. Apply this by hand (DBA / Cloud SQL console)
-- BEFORE deploying the code that depends on it.
ALTER TABLE public.chat_thread
    ADD COLUMN IF NOT EXISTS hidden_at  timestamp without time zone,
    ADD COLUMN IF NOT EXISTS cleared_at timestamp without time zone;
