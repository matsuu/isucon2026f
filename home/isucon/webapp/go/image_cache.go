package isureal

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"

	"github.com/labstack/echo/v5"
)

// imageCacheURLPrefix は nginx の internal location と一致させる。
const imageCacheURLPrefix = "/_internal_images/"

func imageCacheKey(imageID string) string {
	sum := sha256.Sum256([]byte(imageID))
	return hex.EncodeToString(sum[:])
}

func imageFileExtension(mediaType string) string {
	switch mediaType {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	default:
		return ".bin"
	}
}

func (h *HandlerContext) imageCachePath(imageID, mediaType string) string {
	return filepath.Join(h.Config.ImageCacheDir, imageCacheKey(imageID)+imageFileExtension(mediaType))
}

// cachedImagePath はキャッシュファイルが存在し、空でない場合にそのパスを返す。
func (h *HandlerContext) cachedImagePath(imageID, mediaType string) (string, bool) {
	path := h.imageCachePath(imageID, mediaType)
	info, err := os.Stat(path)
	if err != nil {
		return path, false
	}
	return path, info.Mode().IsRegular() && info.Size() > 0
}

// writeImageCacheFile は同一ディレクトリ内の一時ファイルに書いてから rename する。
// nginx が書きかけのファイルを読むことはない。
func writeImageCacheFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".image-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func serveCachedImage(c *echo.Context, path, mediaType string) error {
	c.Response().Header().Set("Content-Type", mediaType)
	c.Response().Header().Set("X-Accel-Redirect", imageCacheURLPrefix+filepath.Base(path))
	return c.NoContent(http.StatusOK)
}

// removeCachedImage は削除された画像のキャッシュを掃除する。失敗してもリクエストは継続する。
func (h *HandlerContext) removeCachedImage(imageID string) {
	matches, err := filepath.Glob(filepath.Join(h.Config.ImageCacheDir, imageCacheKey(imageID)+".*"))
	if err != nil {
		return
	}
	for _, path := range matches {
		_ = os.Remove(path)
	}
}

func (h *HandlerContext) clearImageCache() error {
	if h.Config.ImageCacheDir == "" {
		return nil
	}
	if err := os.RemoveAll(h.Config.ImageCacheDir); err != nil {
		return err
	}
	return os.MkdirAll(h.Config.ImageCacheDir, 0o755)
}
