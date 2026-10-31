package isureal

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"

	json "github.com/goccy/go-json"
	"github.com/labstack/echo/v5"
)

type exportPostResponse struct {
	createPostResponse
	ImagePath string `json:"imagePath"`
}

func exportPostsHandler(h *HandlerContext) echo.HandlerFunc {
	return func(c *echo.Context) error {
		ctx := c.Request().Context()
		userID := GetSession(c).UserID
		author, err := getUser(ctx, h.DB, userID)
		if err != nil {
			return err
		}
		// 画像本体はDBから読まず、preset/実行時キャッシュのローカルファイルを優先する。
		// 見つからない画像だけDBから取得する。
		rows, err := h.DB.QueryxContext(ctx, `
   SELECT `+postColumns+`
   FROM posts p
   JOIN posting_windows w ON w.id = p.posting_window_id
   WHERE p.author_id = $1 AND p.deleted_at IS NULL
   ORDER BY p.posted_at, p.id
  `, userID)
		if err != nil {
			return err
		}
		posts := make([]Post, 0, 64)
		for rows.Next() {
			var post Post
			if err := rows.StructScan(&post); err != nil {
				rows.Close()
				return err
			}
			posts = append(posts, post)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}

		c.Response().Header().Set(echo.HeaderContentDisposition, `attachment; filename="isureal-posts.zip"`)
		c.Response().Header().Set(echo.HeaderCacheControl, "no-store")
		c.Response().WriteHeader(http.StatusOK)

		writer := zip.NewWriter(c.Response())
		profileImageURL := newGetMeResponse(author).ProfileImageURL
		items := make([]exportPostResponse, 0, len(posts))
		type pendingImage struct {
			post      Post
			imagePath string
		}
		pending := make([]pendingImage, 0)
		for i := range posts {
			post := posts[i]
			imagePath := fmt.Sprintf("images/%06d.jpg", len(items)+1)
			items = append(items, exportPostResponse{
				createPostResponse: newPostResponseWithProfileImageURL(post, author, profileImageURL),
				ImagePath:          imagePath,
			})
			if file, ok := h.openImageFile(post.ImageID, "image/jpeg"); ok {
				image, err := writer.CreateHeader(&zip.FileHeader{Name: imagePath, Method: zip.Store})
				if err != nil {
					file.Close()
					return err
				}
				_, err = io.Copy(image, file)
				file.Close()
				if err != nil {
					return err
				}
			} else {
				pending = append(pending, pendingImage{post: post, imagePath: imagePath})
			}
		}

		// ローカルキャッシュにない画像は、1件ずつDBへ問い合わせず50件ずつまとめて取得する。
		for start := 0; start < len(pending); start += 50 {
			end := min(start+50, len(pending))
			ids := make([]string, 0, end-start)
			for _, item := range pending[start:end] {
				ids = append(ids, item.post.ImageID)
			}
			var imageRows []struct {
				ID   string `db:"id"`
				Data []byte `db:"data"`
			}
			if err := h.DB.SelectContext(ctx, &imageRows, `
    SELECT l.id, i.data
    FROM image_object_locations l
    JOIN image_objects i ON i.id = l.id AND i.created_at = l.created_at
    WHERE l.id = ANY($1::text[])
   `, ids); err != nil {
				return err
			}
			imageData := make(map[string][]byte, len(imageRows))
			for _, row := range imageRows {
				imageData[row.ID] = row.Data
			}
			for _, item := range pending[start:end] {
				data, ok := imageData[item.post.ImageID]
				if !ok {
					// mapping に存在しない古い画像だけ、従来どおり単体取得にフォールバックする。
					if err := h.DB.GetContext(ctx, &data, `SELECT i.data FROM image_objects i WHERE i.id=$1 AND i.created_at=$2`, item.post.ImageID, item.post.PostedAt); err != nil {
						return err
					}
				}
				image, err := writer.CreateHeader(&zip.FileHeader{Name: item.imagePath, Method: zip.Store})
				if err != nil {
					return err
				}
				if _, err := image.Write(data); err != nil {
					return err
				}
			}
		}

		manifest, err := writer.Create("posts.json")
		if err != nil {
			return err
		}
		if err := json.NewEncoder(manifest).Encode(items); err != nil {
			return err
		}
		return writer.Close()
	}
}
