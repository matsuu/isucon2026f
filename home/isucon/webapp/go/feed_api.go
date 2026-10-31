package isureal

import (
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"
)

type getFeedRequest struct {
	Feed           string     `param:"feed"`
	BeforeScore    *int64     `query:"beforeScore"`
	BeforePostedAt *time.Time `query:"beforePostedAt"`
	BeforeID       string     `query:"beforeId"`
}

type feedItemResponse struct {
	createPostResponse
	Score *int64 `json:"score,omitempty"`
}

type getFeedResponse struct {
	Items   []feedItemResponse `json:"items"`
	HasMore bool               `json:"hasMore"`
}

func getFeedHandler(h *HandlerContext) echo.HandlerFunc {
	return func(c *echo.Context) error {
		var input getFeedRequest
		if err := c.Bind(&input); err != nil {
			return err
		}
		switch input.Feed {
		case "friends", "friends-of-friends", "public", "following":
		default:
			return NewAPIError(http.StatusBadRequest, "invalid_post")
		}
		if (input.BeforePostedAt == nil) != (input.BeforeID == "") {
			return NewAPIError(http.StatusBadRequest, "invalid_pagination")
		}
		ranked := input.Feed == "friends-of-friends"
		if ranked && (input.BeforeScore == nil) != (input.BeforeID == "") {
			return NewAPIError(http.StatusBadRequest, "invalid_pagination")
		}
		ctx := c.Request().Context()
		viewer, err := loadViewerContext(ctx, h.DB, GetSession(c).UserID)
		if errors.Is(err, sql.ErrNoRows) {
			return c.JSON(http.StatusOK, getFeedResponse{Items: []feedItemResponse{}})
		}
		if err != nil {
			return err
		}
		if !viewer.HasPostedToday {
			return c.JSON(http.StatusOK, getFeedResponse{Items: []feedItemResponse{}})
		}

		followingIDs := make([]string, 0)
		if input.Feed == "following" {
			if err := h.DB.SelectContext(ctx, &followingIDs, `SELECT followee_id FROM follows WHERE follower_id = $1`, viewer.ID); err != nil {
				return err
			}
			if followingIDs == nil {
				followingIDs = []string{}
			}
		}

		posts, err := selectFeedPosts(ctx, h.DB, viewer, input.Feed, followingIDs, input.BeforePostedAt, input.BeforeID, input.BeforeScore)
		if err != nil {
			return err
		}

		items := make([]feedItemResponse, 0, len(posts))
		profileImageURLs := make(map[string]*string)
		for _, post := range posts {
			profileImageURL, ok := profileImageURLs[post.AuthorID]
			if !ok {
				profileImageURL = newGetMeResponse(post.Author).ProfileImageURL
				profileImageURLs[post.AuthorID] = profileImageURL
			}
			item := feedItemResponse{createPostResponse: newPostResponseWithProfileImageURL(post.Post, post.Author, profileImageURL)}
			if ranked {
				score := post.Score
				item.Score = &score
			}
			items = append(items, item)
		}
		hasMore := len(items) > 50
		if hasMore {
			items = items[:50]
		}
		return c.JSON(http.StatusOK, getFeedResponse{Items: items, HasMore: hasMore})
	}
}
