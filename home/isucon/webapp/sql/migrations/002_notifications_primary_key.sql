-- notifications.id はアプリから検索されておらず、id のPKインデックスが
-- 約147MBで未使用だった。実際の一意キー (user_id, posting_window_id) を
-- PKに置き換えて、通知一括INSERTのインデックス更新を減らす。
--
-- 001-schema.sql から新規作成したDBでは既に新しいPKになっているため、
-- このmigrationは既存DB向け。何度実行しても新しいPKに収束する。
ALTER TABLE notifications DROP CONSTRAINT IF EXISTS notifications_pkey;
ALTER TABLE notifications DROP CONSTRAINT IF EXISTS notifications_user_id_posting_window_id_key;
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid = 'notifications'::regclass AND contype = 'p'
  ) THEN
    ALTER TABLE notifications ADD PRIMARY KEY (user_id, posting_window_id);
  END IF;
END $$;
