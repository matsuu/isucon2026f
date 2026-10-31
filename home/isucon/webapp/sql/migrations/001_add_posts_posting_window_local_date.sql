-- createPostHandler のリージョン全体FOR UPDATEを避けるための migration。
--
-- 既存DBに適用する場合は、CREATE INDEX を CREATE INDEX CONCURRENTLY に
-- 置き換えてトランザクション外で実行する。
--
-- (author_id, posting_window_local_date) の UNIQUE は「削除済み投稿を含む全投稿」に
-- 対して張ると、削除後の再投稿や「削除2回まで」の仕様を壊す。
-- そのため active 投稿だけを対象にした部分UNIQUEインデックスにする。
-- deleted件数の上限判定はアプリ側に残す。

ALTER TABLE posts
  ADD COLUMN IF NOT EXISTS posting_window_local_date date;

UPDATE posts p
SET posting_window_local_date = w.local_date
FROM posting_windows w
WHERE w.id = p.posting_window_id
  AND p.posting_window_local_date IS NULL;

-- deleted件数カウント用。active用の部分UNIQUEでも先頭列は使えるが、
-- 削除済み投稿も含めた検索用に通常の複合インデックスも用意する。
CREATE INDEX IF NOT EXISTS idx_posts_author_local_date
  ON posts(author_id, posting_window_local_date);

-- 同じユーザー・同じ日付の active 投稿は1件だけ許可する。
CREATE UNIQUE INDEX IF NOT EXISTS idx_posts_author_local_date_unique
  ON posts(author_id, posting_window_local_date)
  WHERE deleted_at IS NULL;
