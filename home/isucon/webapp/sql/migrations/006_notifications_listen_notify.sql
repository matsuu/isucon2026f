-- notifications の一括INSERT後に1回だけ pg_notify する。
-- 行ごとに通知すると29,000行INSERTで29,000回のNOTIFYになり重いため、
-- statement-level trigger の遷移テーブルから posting_window_id を1つ取り出す。
CREATE OR REPLACE FUNCTION isureal_notify_notifications() RETURNS trigger AS $$
DECLARE
  window_id text;
BEGIN
  SELECT posting_window_id INTO window_id FROM new_rows LIMIT 1;
  IF window_id IS NOT NULL THEN
    PERFORM pg_notify('isureal_notifications', window_id);
  END IF;
  RETURN NULL;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS notifications_notify ON notifications;
CREATE TRIGGER notifications_notify
AFTER INSERT ON notifications
REFERENCING NEW TABLE AS new_rows
FOR EACH STATEMENT
EXECUTE FUNCTION isureal_notify_notifications();
