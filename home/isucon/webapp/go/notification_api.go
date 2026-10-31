package isureal

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	json "github.com/goccy/go-json"
	"github.com/labstack/echo/v5"
)

type getNotificationsResponse struct {
	ID              string    `json:"id" db:"id"`
	PostingWindowID string    `json:"postingWindowId" db:"posting_window_id"`
	Region          string    `json:"region" db:"region"`
	CreatedAt       time.Time `json:"createdAt" db:"created_at"`
}

func getNotificationsHandler(h *HandlerContext) echo.HandlerFunc {
	return func(c *echo.Context) error {
		ctx := c.Request().Context()
		session := GetSession(c)
		userID := session.UserID
		response := c.Response()
		response.Header().Set("Content-Type", "text/event-stream")
		response.WriteHeader(http.StatusOK)
		controller := http.NewResponseController(response)
		if err := controller.Flush(); err != nil {
			return nil
		}
		// LISTEN/NOTIFYのイベントで起床する。LISTEN接続はアプリ全体で1本。
		// 1秒tickerはLISTEN切断時のフォールバック。接続時点で現在枠の通知が
		// 既に作られていても、その通知を返す。
		var events <-chan struct{}
		if h.Notifications != nil {
			var unsubscribe func()
			events, unsubscribe = h.Notifications.Subscribe()
			defer unsubscribe()
		}
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			var row struct {
				UserRegion      string     `db:"user_region"`
				ID              *string    `db:"id"`
				PostingWindowID *string    `db:"posting_window_id"`
				Region          *string    `db:"region"`
				CreatedAt       *time.Time `db:"created_at"`
			}
			err := h.DB.GetContext(ctx, &row, `
   SELECT u.region AS user_region, latest.id, latest.posting_window_id, latest.region, latest.created_at
   FROM users u
   LEFT JOIN LATERAL (
     SELECT n.id, n.posting_window_id, w.region, n.created_at
     FROM notifications n
     JOIN posting_windows w ON w.id = n.posting_window_id
     WHERE n.user_id = u.id
       AND w.ended_at IS NULL
     ORDER BY n.created_at, n.id
     LIMIT 1
   ) latest ON true
   WHERE u.id = $1 AND u.status = 'active'
  `, userID)
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			if err != nil {
				if ctx.Err() == nil {
					c.Logger().Error("failed to get notification", "error", err)
				}
				return nil
			}
			if row.UserRegion != session.Region {
				return nil
			}
			if row.ID == nil || row.PostingWindowID == nil || row.Region == nil || row.CreatedAt == nil {
				select {
				case <-ctx.Done():
					return nil
				case <-events:
				case <-ticker.C:
				}
				continue
			}
			notification := getNotificationsResponse{
				ID:              *row.ID,
				PostingWindowID: *row.PostingWindowID,
				Region:          *row.Region,
				CreatedAt:       *row.CreatedAt,
			}
			data, err := json.Marshal(notification)
			if err != nil {
				c.Logger().Error("failed to encode notification", "error", err)
				return nil
			}
			if _, err := fmt.Fprintf(response, "data: %s\n\n", data); err != nil {
				return nil
			}
			_ = controller.Flush()
			return nil
		}
	}
}
