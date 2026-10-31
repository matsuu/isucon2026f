package isureal

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

type feedPostWithAuthor struct {
	PostWithAuthor
	Score int64 `db:"score"`
}

func mapKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func mutualFriendSlices(values map[string]int) ([]string, []int) {
	ids := make([]string, 0, len(values))
	counts := make([]int, 0, len(values))
	for id, count := range values {
		ids = append(ids, id)
		counts = append(counts, count)
	}
	return ids, counts
}

// selectFeedPosts は feed の可視性・feed種別・ページングをSQL側で処理する。
// 戻り値は最大51件で、呼び出し側で50件に切り詰めて hasMore を判定する。
func selectFeedPosts(
	ctx context.Context,
	db sqlx.QueryerContext,
	viewer viewerContext,
	feed string,
	followingIDs []string,
	beforePostedAt *time.Time,
	beforeID string,
	beforeScore *int64,
) ([]feedPostWithAuthor, error) {
	var friendIDs, fofIDs, followingArg []string
	switch feed {
	case "friends":
		friendIDs = mapKeys(viewer.Friends)
	case "following":
		friendIDs = mapKeys(viewer.Friends)
		fofIDs = mapKeys(viewer.FriendsOfFriends)
		followingArg = followingIDs
	case "friends-of-friends":
		friendIDs = mapKeys(viewer.Friends)
		fofIDs = mapKeys(viewer.FriendsOfFriends)
	}
	blockedIDs := mapKeys(viewer.Blocked)
	mutualIDs, mutualCounts := mutualFriendSlices(viewer.MutualFriends)

	conditions := []string{
		`p.deleted_at IS NULL`,
		`w.ended_at IS NULL`,
		`u.status = 'active'`,
		`p.author_id <> $1`,
		`NOT (p.author_id = ANY($4::text[]))`,
		// feed種別によっては使わない配列パラメータの型を確定させる。
		`COALESCE($2::text[], '{}'::text[]) IS NOT NULL`,
		`COALESCE($3::text[], '{}'::text[]) IS NOT NULL`,
		`COALESCE($5::text[], '{}'::text[]) IS NOT NULL`,
	}
	switch feed {
	case "public":
		conditions = append(conditions, `p.visibility = 'public'`)
	case "friends":
		conditions = append(conditions,
			`p.author_id = ANY($2::text[])`,
			`p.visibility IN ('public', 'friends', 'friends_of_friends')`,
		)
	case "following":
		conditions = append(conditions,
			`p.author_id = ANY($5::text[])`,
			`(p.visibility = 'public'
			  OR (p.visibility = 'friends' AND p.author_id = ANY($2::text[]))
			  OR (p.visibility = 'friends_of_friends' AND (p.author_id = ANY($2::text[]) OR p.author_id = ANY($3::text[]))))`,
		)
	case "friends-of-friends":
		conditions = append(conditions,
			`p.author_id = ANY($3::text[])`,
			`NOT (p.author_id = ANY($2::text[]))`,
			`p.visibility IN ('public', 'friends_of_friends')`,
		)
	default:
		return nil, fmt.Errorf("invalid feed: %s", feed)
	}

	candidates := `
  SELECT p.id, p.author_id, p.posting_window_id, p.image_id, p.caption, p.visibility,
         p.location_kind, p.latitude, p.longitude, p.locality, p.posted_at,
         w.on_time_deadline, w.ended_at AS window_ended_at,
         ` + userColumnsAs("u", "author") + `
  FROM posts p
  JOIN posting_windows w ON w.id = p.posting_window_id
  JOIN users u ON u.id = p.author_id AND u.status = 'active'
  WHERE ` + strings.Join(conditions, " AND ")

	baseArgs := []any{viewer.ID, friendIDs, fofIDs, blockedIDs, followingArg}

	var posts []feedPostWithAuthor
	if feed != "friends-of-friends" {
		args := append(append([]any{}, baseArgs...), beforePostedAt, beforeID)
		query := `WITH candidates AS (` + candidates + `)
SELECT *, 0::bigint AS score
FROM candidates
WHERE ($6::timestamptz IS NULL OR (posted_at, id) < ($6, $7))
ORDER BY posted_at DESC, id DESC
LIMIT 51`
		if err := sqlx.SelectContext(ctx, db, &posts, query, args...); err != nil {
			return nil, err
		}
	} else {
		args := append(append([]any{}, baseArgs...), mutualIDs, mutualCounts, beforeScore, beforePostedAt, beforeID)
		query := `WITH candidates AS (` + candidates + `),
mutual_friends AS (
  SELECT * FROM unnest($6::text[], $7::int[]) AS m(user_id, cnt)
),
connections AS (
  SELECT ca.author_id, sum(mf.cnt)::bigint AS cnt
  FROM (SELECT DISTINCT author_id FROM candidates) ca
  JOIN friendships f ON ca.author_id IN (f.user_low_id, f.user_high_id)
  JOIN mutual_friends mf ON mf.user_id = CASE
    WHEN f.user_low_id = ca.author_id THEN f.user_high_id
    ELSE f.user_low_id
  END
  GROUP BY ca.author_id
),
friend_engagements AS (
  SELECT e.post_id, count(*)::bigint AS cnt
  FROM (
    SELECT r.post_id, r.reactor_id AS user_id
    FROM realmojis r
    JOIN users ru ON ru.id = r.reactor_id AND ru.status = 'active'
    WHERE r.replaced_at IS NULL AND r.deleted_at IS NULL
      AND r.post_id IN (SELECT id FROM candidates)
    UNION
    SELECT c.post_id, c.author_id
    FROM comments c
    JOIN users cu ON cu.id = c.author_id AND cu.status = 'active'
    WHERE c.deleted_at IS NULL
      AND c.post_id IN (SELECT id FROM candidates)
  ) e
  JOIN unnest($2::text[]) AS fr(user_id) ON fr.user_id = e.user_id
  GROUP BY e.post_id
),
scored AS (
  SELECT c.*,
         10 * COALESCE(fe.cnt, 0) + 3 * COALESCE(mf.cnt, 0) + LEAST(COALESCE(conn.cnt, 0), 30) AS score
  FROM candidates c
  LEFT JOIN mutual_friends mf ON mf.user_id = c.author_id
  LEFT JOIN connections conn ON conn.author_id = c.author_id
  LEFT JOIN friend_engagements fe ON fe.post_id = c.id
)
SELECT *
FROM scored
WHERE ($8::bigint IS NULL OR (score, posted_at, id) < ($8, $9, $10))
ORDER BY score DESC, posted_at DESC, id DESC
LIMIT 51`
		if err := sqlx.SelectContext(ctx, db, &posts, query, args...); err != nil {
			return nil, err
		}
	}

	for i := range posts {
		if err := posts[i].Author.setImageKey(); err != nil {
			return nil, err
		}
	}
	return posts, nil
}
