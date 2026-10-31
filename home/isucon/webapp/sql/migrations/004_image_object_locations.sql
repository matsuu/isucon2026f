-- image_objects は created_at でパーティション分割されている。
-- 画像IDだけの検索で全パーティションを走査しないよう、id -> created_at の
-- 非パーティション索引テーブルを追加する。
CREATE TABLE IF NOT EXISTS image_object_locations (
  id varchar(36) PRIMARY KEY,
  created_at timestamptz NOT NULL
);

INSERT INTO image_object_locations (id, created_at)
SELECT id, created_at FROM image_objects
ON CONFLICT (id) DO NOTHING;
