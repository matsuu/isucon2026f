-- 管理者の投稿枠オープン時は notifications を約29,000行一括INSERTする。
-- SSEは「現在枠の通知がある」ことだけ分かればよいので、遷移テーブルを
-- 使わず statement-level trigger で1回だけ pg_notify する。
CREATE OR REPLACE FUNCTION isureal_notify_notifications() RETURNS trigger AS $$
BEGIN
  PERFORM pg_notify('isureal_notifications', '');
  RETURN NULL;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS notifications_notify ON notifications;
CREATE TRIGGER notifications_notify
AFTER INSERT ON notifications
FOR EACH STATEMENT
EXECUTE FUNCTION isureal_notify_notifications();

-- 現在枠の通知取得はPK(user_id, posting_window_id)でuser_idを引ける。
-- 236MBの索引更新をなくして管理者の一括INSERTを軽くする。
DROP INDEX CONCURRENTLY IF EXISTS notifications_user_created_at_id_idx;
