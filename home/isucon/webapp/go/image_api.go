package isureal

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/labstack/echo/v5"
)

type getImageRequest struct {
	ImageID string `param:"imageId" json:"-"`
	Exp     string `query:"exp" json:"-"`
	Sig     string `query:"sig" json:"-"`
}

const imageVisibilityByOwnerSQL = `(
     COALESCE(u.profile_image_id = $1, false)
     OR EXISTS (
       SELECT 1 FROM posts p
       WHERE p.image_id = $1 AND p.author_id = u.id AND p.deleted_at IS NULL
     )
     OR EXISTS (
       SELECT 1 FROM realmojis r
       JOIN posts p ON p.id = r.post_id AND p.deleted_at IS NULL
       JOIN users author ON author.id = p.author_id AND author.status = 'active'
       WHERE r.image_id = $1 AND r.reactor_id = u.id AND r.replaced_at IS NULL AND r.deleted_at IS NULL
     )
   )`

const imageVisibilitySQL = `(
     COALESCE(u.profile_image_id = i.id, false)
     OR EXISTS (
       SELECT 1 FROM posts p
       WHERE p.image_id = i.id AND p.author_id = u.id AND p.deleted_at IS NULL
     )
     OR EXISTS (
       SELECT 1 FROM realmojis r
       JOIN posts p ON p.id = r.post_id AND p.deleted_at IS NULL
       JOIN users author ON author.id = p.author_id AND author.status = 'active'
       WHERE r.image_id = i.id AND r.reactor_id = u.id AND r.replaced_at IS NULL AND r.deleted_at IS NULL
     )
   )`

const imageVisibilityCacheTTL = 5 * time.Second

type imageMetadata struct {
	OwnerUserID   string
	SecretKeyHash string
	MediaType     string
	Visible       bool
	VisibleUntil  time.Time
}

// 画像ID -> owner/署名鍵/media type は不変なのでプロセス内でキャッシュする。
// 可視性も画像削除の5秒猶予に合わせて最大5秒だけキャッシュする。
var imageMetadataCache sync.Map

func getImageHandler(h *HandlerContext) echo.HandlerFunc {
	return func(c *echo.Context) error {
		// 画像URLは署名付きで、削除後5秒以内の非公開を維持するためmax-age=5にする。
		c.Response().Header().Set("Cache-Control", "max-age=5")
		var input getImageRequest
		if err := c.Bind(&input); err != nil {
			return NewAPIError(http.StatusForbidden, "image_forbidden")
		}
		query, err := url.ParseQuery(c.Request().URL.RawQuery)
		if err != nil || len(query["exp"]) != 1 || len(query["sig"]) != 1 {
			return NewAPIError(http.StatusForbidden, "image_forbidden")
		}
		exp, err := strconv.ParseInt(input.Exp, 10, 64)
		if err != nil || strconv.FormatInt(exp, 10) != input.Exp || exp <= time.Now().Unix() {
			return NewAPIError(http.StatusForbidden, "image_forbidden")
		}
		sig, err := base64.RawURLEncoding.Strict().DecodeString(input.Sig)
		if err != nil || len(sig) != sha256.Size || base64.RawURLEncoding.EncodeToString(sig) != input.Sig {
			return NewAPIError(http.StatusForbidden, "image_forbidden")
		}

		ctx := c.Request().Context()
		metaValue, cachedMeta := imageMetadataCache.Load(input.ImageID)
		var meta imageMetadata
		var visible bool
		now := time.Now()
		if cachedMeta {
			meta = metaValue.(imageMetadata)
			// falseはキャッシュしない。画像が後から投稿・プロフィール・RealMojiに
			// 使われた場合のwrite-read整合性を崩さないため。
			if meta.Visible && now.Before(meta.VisibleUntil) {
				visible = true
			} else {
				var row struct {
					Visible bool `db:"visible"`
				}
				err = h.DB.GetContext(ctx, &row, `
   SELECT `+imageVisibilityByOwnerSQL+` AS visible
   FROM users u
   WHERE u.id = $2 AND u.status = 'active'
  `, input.ImageID, meta.OwnerUserID)
				if errors.Is(err, sql.ErrNoRows) {
					return NewAPIError(http.StatusForbidden, "image_forbidden")
				}
				if err != nil {
					return err
				}
				visible = row.Visible
				meta.Visible = row.Visible
				if row.Visible {
					meta.VisibleUntil = now.Add(imageVisibilityCacheTTL)
				} else {
					meta.VisibleUntil = time.Time{}
				}
				imageMetadataCache.Store(input.ImageID, meta)
			}
		} else {
			var image struct {
				OwnerUserID   string `db:"owner_user_id"`
				SecretKeyHash string `db:"secret_key_hash"`
				MediaType     string `db:"media_type"`
				Visible       bool   `db:"visible"`
			}
			err = h.DB.GetContext(ctx, &image, `
   SELECT u.id AS owner_user_id, u.secret_key_hash, i.media_type, `+imageVisibilitySQL+` AS visible
   FROM image_object_locations l
   JOIN image_objects i ON i.id = l.id AND i.created_at = l.created_at
   JOIN users u ON u.id = i.owner_user_id
   WHERE l.id = $1 AND u.status = 'active'
  `, input.ImageID)
			if errors.Is(err, sql.ErrNoRows) {
				return NewAPIError(http.StatusForbidden, "image_forbidden")
			}
			if err != nil {
				return err
			}
			meta = imageMetadata{
				OwnerUserID: image.OwnerUserID, SecretKeyHash: image.SecretKeyHash, MediaType: image.MediaType,
				Visible: image.Visible,
			}
			if image.Visible {
				meta.VisibleUntil = now.Add(imageVisibilityCacheTTL)
			}
			imageMetadataCache.Store(input.ImageID, meta)
			visible = image.Visible
		}
		key, err := imageSigningKey(meta.SecretKeyHash)
		if err != nil {
			return err
		}
		if !hmac.Equal(sig, imageSignature(key, input.ImageID, exp)) {
			c.Set("abuse", "invalid_signature")
			return NewAPIError(http.StatusForbidden, "image_forbidden")
		}
		if !visible {
			return NewAPIError(http.StatusForbidden, "image_forbidden")
		}
		if exp <= time.Now().Unix() {
			return NewAPIError(http.StatusForbidden, "image_forbidden")
		}

		// reset.sql で消えない初期画像は、事前生成した preset を優先して使う。
		if presetPath, ok := h.cachedPresetImagePath(input.ImageID, meta.MediaType); ok {
			return h.servePresetImage(c, presetPath, meta.MediaType)
		}

		cachePath, cached := h.cachedImagePath(input.ImageID, meta.MediaType)
		if cached {
			return h.serveCachedImage(c, cachePath, meta.MediaType)
		}

		// キャッシュヒット時も最初のクエリで削除状態を確認済み。
		// ミス時はデータ取得クエリでも再確認する。即時反映も5秒以内の許容範囲に含まれる。
		var object struct {
			Data []byte `db:"data"`
		}
		err = h.DB.GetContext(ctx, &object, `
   SELECT i.data
   FROM image_object_locations l
   JOIN image_objects i ON i.id = l.id AND i.created_at = l.created_at
   JOIN users u ON u.id = i.owner_user_id AND u.status = 'active'
   WHERE l.id = $1 AND `+imageVisibilitySQL+`
  `, input.ImageID)
		if errors.Is(err, sql.ErrNoRows) {
			return NewAPIError(http.StatusForbidden, "image_forbidden")
		}
		if err != nil {
			return err
		}
		if exp <= time.Now().Unix() {
			return NewAPIError(http.StatusForbidden, "image_forbidden")
		}
		if err := writeImageCacheFile(cachePath, object.Data); err != nil {
			// キャッシュに書けなくても画像自体は返す。
			return c.Blob(http.StatusOK, meta.MediaType, object.Data)
		}
		h.rememberRuntimeImagePath(input.ImageID, meta.MediaType, cachePath)
		return h.serveCachedImage(c, cachePath, meta.MediaType)
	}
}
