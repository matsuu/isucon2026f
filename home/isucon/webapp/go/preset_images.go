package isureal

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jmoiron/sqlx"
)

type PresetImageOptions struct {
	Dir         string
	Workers     int
	Limit       int
	Force       bool
	VisibleOnly bool
	Logger      *log.Logger
}

type presetImageJob struct {
	ID        string
	MediaType string
	Data      []byte
}

type presetImageRef struct {
	ID        string `db:"id"`
	MediaType string `db:"media_type"`
}

func listPresetImageRefs(ctx context.Context, db *sqlx.DB, opts PresetImageOptions, boundary time.Time) ([]presetImageRef, error) {
	query := `SELECT i.id, i.media_type FROM image_objects i`
	args := []any{boundary}
	if opts.VisibleOnly {
		query += ` JOIN users u ON u.id = i.owner_user_id AND u.status = 'active'`
		query += ` WHERE i.created_at <= $1 AND ` + imageVisibilitySQL
	} else {
		query += ` WHERE i.created_at <= $1`
	}
	if opts.Limit > 0 {
		query += ` LIMIT $2`
		args = append(args, opts.Limit)
	}
	var refs []presetImageRef
	if err := db.SelectContext(ctx, &refs, query, args...); err != nil {
		return nil, err
	}
	return refs, nil
}

// GeneratePresetImages は reset.sql で消えない初期画像をファイルとして出力する。
// 既存ファイルは -Force 指定時だけ上書きするので、中断後の再実行で続きから作れる。
func GeneratePresetImages(ctx context.Context, db *sqlx.DB, opts PresetImageOptions) error {
	logger := opts.Logger
	if logger == nil {
		logger = log.Default()
	}
	if opts.Dir == "" {
		return fmt.Errorf("preset image directory is empty")
	}
	workers := opts.Workers
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	if err := os.MkdirAll(opts.Dir, 0o755); err != nil {
		return err
	}
	// ファイル名の先頭2文字でディレクトリを分ける。大量のファイルを1ディレクトリに置かない。
	for i := 0; i < 256; i++ {
		if err := os.MkdirAll(filepath.Join(opts.Dir, fmt.Sprintf("%02x", i)), 0o755); err != nil {
			return err
		}
	}

	var boundary time.Time
	if err := db.GetContext(ctx, &boundary, `SELECT created_at FROM initial_data_boundary`); err != nil {
		return fmt.Errorf("get initial_data_boundary: %w", err)
	}

	refs, err := listPresetImageRefs(ctx, db, opts, boundary)
	if err != nil {
		return err
	}

	var written, skipped int64
	if !opts.Force {
		missing := refs[:0]
		for _, ref := range refs {
			if _, ok := existingImagePath(imageFilePath(opts.Dir, ref.ID, ref.MediaType)); ok {
				atomic.AddInt64(&skipped, 1)
				continue
			}
			missing = append(missing, ref)
		}
		refs = missing
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	jobs := make(chan presetImageJob, workers*2)
	var wg sync.WaitGroup
	var firstErr error
	var errOnce sync.Once
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				path := imageFilePath(opts.Dir, job.ID, job.MediaType)
				if !opts.Force {
					if _, ok := existingImagePath(path); ok {
						atomic.AddInt64(&skipped, 1)
						continue
					}
				}
				if err := writeImageFile(path, job.Data); err != nil {
					errOnce.Do(func() {
						firstErr = err
						cancel()
					})
					return
				}
				n := atomic.AddInt64(&written, 1)
				if n%10000 == 0 {
					logger.Printf("preset images: written=%d skipped=%d", n, atomic.LoadInt64(&skipped))
				}
			}
		}()
	}

	var readErr error
	var read int64
	const batchSize = 500
loop:
	for start := 0; start < len(refs); start += batchSize {
		end := min(start+batchSize, len(refs))
		ids := make([]string, 0, end-start)
		for _, ref := range refs[start:end] {
			ids = append(ids, ref.ID)
		}
		rows, err := db.QueryxContext(runCtx, `
   SELECT l.id, i.media_type, i.data
   FROM image_object_locations l
   JOIN image_objects i ON i.id = l.id AND i.created_at = l.created_at
   WHERE l.id = ANY($1::text[])
  `, ids)
		if err != nil {
			readErr = err
			break
		}
		for rows.Next() {
			var job presetImageJob
			if err := rows.Scan(&job.ID, &job.MediaType, &job.Data); err != nil {
				readErr = err
				break
			}
			select {
			case <-runCtx.Done():
				rows.Close()
				break loop
			case jobs <- job:
				read++
			}
		}
		if readErr == nil {
			readErr = rows.Err()
		}
		rows.Close()
		if readErr != nil {
			break
		}
	}
	close(jobs)
	wg.Wait()

	if readErr != nil {
		return readErr
	}
	if firstErr != nil {
		return firstErr
	}
	if err := runCtx.Err(); err != nil {
		return err
	}
	logger.Printf("preset images: done written=%d skipped=%d read=%d dir=%s", atomic.LoadInt64(&written), atomic.LoadInt64(&skipped), read, opts.Dir)
	return nil
}
