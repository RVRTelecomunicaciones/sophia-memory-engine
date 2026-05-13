-- Memory Engine — P1.3 down: drop the partial unique index on active topic_keys.
--
-- The Step-1 backfill in the up migration is NOT reverted; archived rows
-- marked by the backfill remain archived. That is intentional: rolling back
-- the index does not invalidate the supersession decisions already recorded.

DROP INDEX IF EXISTS idx_memories_topic_key_active_unique;
