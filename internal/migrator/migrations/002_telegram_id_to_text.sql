-- Migration 002: change telegram_id column type from BIGINT to TEXT
-- to support lookup by Telegram @username instead of numeric ID.
ALTER TABLE users
ALTER COLUMN telegram_id TYPE TEXT USING telegram_id::TEXT;