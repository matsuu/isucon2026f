-- 一覧系の「親ID + 並び順」をインデックスだけで処理するための複合インデックス。
-- 既存DBでは CREATE INDEX CONCURRENTLY で適用する。

CREATE INDEX IF NOT EXISTS comments_post_id_created_at_id_idx
  ON comments(post_id, created_at, id)
  WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS realmojis_post_id_created_at_id_idx
  ON realmojis(post_id, created_at DESC, id DESC)
  WHERE replaced_at IS NULL AND deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS posts_author_posted_at_id_idx
  ON posts(author_id, posted_at DESC, id DESC)
  WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS posts_visibility_posted_at_id_idx
  ON posts(visibility, posted_at DESC, id DESC)
  WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS notifications_user_created_at_id_idx
  ON notifications(user_id, created_at, id);
