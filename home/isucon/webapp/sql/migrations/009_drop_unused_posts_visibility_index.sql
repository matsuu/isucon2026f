-- 公開フィード用に追加したが、実測でidx_scan=0だった。
-- 投稿INSERT時のインデックス更新を減らすため削除する。
DROP INDEX CONCURRENTLY IF EXISTS posts_visibility_posted_at_id_idx;
