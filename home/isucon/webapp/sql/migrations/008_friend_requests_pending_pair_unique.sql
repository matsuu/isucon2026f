-- 同じ2ユーザー間にpendingの友達申請が複数作られないことをDBでも保証する。
-- アプリ側はペアadvisory lockで直列化しているが、防御的にUNIQUEを追加する。
CREATE UNIQUE INDEX IF NOT EXISTS friend_requests_pending_pair_unique
  ON friend_requests (LEAST(requester_id, recipient_id), GREATEST(requester_id, recipient_id))
  WHERE status = 'pending';
