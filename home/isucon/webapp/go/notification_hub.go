package isureal

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
)

const notificationChannel = "isureal_notifications"

// NotificationHub は1つのLISTEN接続で受けた通知を、同じアプリサーバー内の
// SSEハンドラへ配信する。SSE接続ごとにDB接続を保持しないための仕組み。
type NotificationHub struct {
	mu          sync.Mutex
	subscribers map[chan struct{}]struct{}
}

func NewNotificationHub() *NotificationHub {
	return &NotificationHub{subscribers: make(map[chan struct{}]struct{})}
}

func (h *NotificationHub) Subscribe() (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	h.mu.Lock()
	h.subscribers[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		delete(h.subscribers, ch)
		h.mu.Unlock()
	}
}

func (h *NotificationHub) Publish() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subscribers {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// startNotificationListener はPostgreSQLのLISTEN接続を1本だけ維持し、
// 切断時は再接続する。通知はDBコミット後に届く。
func startNotificationListener(ctx context.Context, dsn string, hub *NotificationHub) {
	go func() {
		for {
			err := listenNotifications(ctx, dsn, hub)
			if ctx.Err() != nil {
				return
			}
			log.Printf("notification listener disconnected: %v", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
		}
	}()
}

func listenNotifications(ctx context.Context, dsn string, hub *NotificationHub) error {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return err
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = conn.Close(closeCtx)
	}()

	if _, err := conn.Exec(ctx, "LISTEN "+notificationChannel); err != nil {
		return err
	}
	for {
		if _, err := conn.WaitForNotification(ctx); err != nil {
			return err
		}
		hub.Publish()
	}
}
