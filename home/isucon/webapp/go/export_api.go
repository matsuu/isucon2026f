package isureal

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

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
		defer rows.Close()

		c.Response().Header().Set(echo.HeaderContentDisposition, `attachment; filename="isureal-posts.zip"`)
		c.Response().Header().Set(echo.HeaderCacheControl, "no-store")
		c.Response().WriteHeader(http.StatusOK)

		writer := zip.NewWriter(c.Response())
		profileImageURL := newGetMeResponse(author).ProfileImageURL
		items := make([]exportPostResponse, 0)
		for rows.Next() {
			var post Post
			if err := rows.StructScan(&post); err != nil {
				return err
			}
			imagePath := fmt.Sprintf("images/%06d.jpg", len(items)+1)
			image, err := writer.CreateHeader(&zip.FileHeader{Name: imagePath, Method: zip.Store})
			if err != nil {
				return err
			}
			if file, ok := h.openImageFile(post.ImageID, "image/jpeg"); ok {
				_, err = io.Copy(image, file)
				file.Close()
				if err != nil {
					return err
				}
			} else {
				var imageData []byte
				if err := h.DB.GetContext(ctx, &imageData, `SELECT i.data FROM image_objects i WHERE i.id=$1 AND i.created_at=$2`, post.ImageID, post.PostedAt); err != nil {
					return err
				}
				if _, err := image.Write(imageData); err != nil {
					return err
				}
			}
			items = append(items, exportPostResponse{
				createPostResponse: newPostResponseWithProfileImageURL(post, author, profileImageURL),
				ImagePath:          imagePath,
			})
		}
		if err := rows.Err(); err != nil {
			return err
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
