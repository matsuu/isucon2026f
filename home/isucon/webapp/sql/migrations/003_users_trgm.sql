-- ユーザー検索の ILIKE '%q%' を高速化する。
-- 既存DBに適用する場合は CREATE INDEX CONCURRENTLY を使い、
-- トランザクション外で実行する。
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX IF NOT EXISTS users_username_trgm_idx
  ON users USING gin (username gin_trgm_ops);

CREATE INDEX IF NOT EXISTS users_display_name_trgm_idx
  ON users USING gin (display_name gin_trgm_ops);
