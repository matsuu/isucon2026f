-- 投稿枠オープン時の notifications 一括INSERTを軽くするため、
-- 挿入元で保証済みの外部キー制約を外す。
ALTER TABLE notifications DROP CONSTRAINT IF EXISTS notifications_user_id_fkey;
ALTER TABLE notifications DROP CONSTRAINT IF EXISTS notifications_posting_window_id_fkey;
