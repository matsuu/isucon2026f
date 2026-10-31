package isureal

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/labstack/echo/v5"
)

// imageCacheURLPrefix / presetImageURLPrefix は nginx の internal location と一致させる。
const imageCacheURLPrefix = "/_internal_images/"
const presetImageURLPrefix = "/_preset_images/"

var imageCacheKeyCache sync.Map
var presetImagePathCache sync.Map
var runtimeImagePathCache sync.Map

func imageCacheKey(imageID string) string {
	if value, ok := imageCacheKeyCache.Load(imageID); ok {
		return value.(string)
	}
	sum := sha256.Sum256([]byte(imageID))
	key := hex.EncodeToString(sum[:])
	imageCacheKeyCache.Store(imageID, key)
	return key
}

func imagePathCacheKey(imageID, mediaType string) string {
	return imageID + "\x00" + mediaType
}

func imageFilePath(dir, imageID, mediaType string) string {
	key := imageCacheKey(imageID)
	return filepath.Join(dir, key[:2], key+imageFileExtension(mediaType))
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
	return imageFilePath(h.Config.ImageCacheDir, imageID, mediaType)
}

func (h *HandlerContext) presetImagePath(imageID, mediaType string) string {
	return imageFilePath(h.Config.PresetImageDir, imageID, mediaType)
}

func existingImagePath(path string) (string, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return path, false
	}
	return path, info.Mode().IsRegular() && info.Size() > 0
}

// cachedImagePath はキャッシュファイルが存在し、空でない場合にそのパスを返す。
// 実行時キャッシュは削除時に明示的に無効化する。
func (h *HandlerContext) cachedImagePath(imageID, mediaType string) (string, bool) {
	cacheKey := imagePathCacheKey(imageID, mediaType)
	if value, ok := runtimeImagePathCache.Load(cacheKey); ok {
		return value.(string), true
	}
	path, ok := existingImagePath(h.imageCachePath(imageID, mediaType))
	if ok {
		runtimeImagePathCache.Store(cacheKey, path)
	}
	return path, ok
}

// cachedPresetImagePath は事前生成済みの preset ファイルが存在する場合にそのパスを返す。
// presetは初期化でも削除されないため、存在確認結果をそのままキャッシュできる。
func (h *HandlerContext) cachedPresetImagePath(imageID, mediaType string) (string, bool) {
	cacheKey := imagePathCacheKey(imageID, mediaType)
	if value, ok := presetImagePathCache.Load(cacheKey); ok {
		return value.(string), true
	}
	path, ok := existingImagePath(h.presetImagePath(imageID, mediaType))
	if ok {
		presetImagePathCache.Store(cacheKey, path)
	}
	return path, ok
}

func (h *HandlerContext) rememberRuntimeImagePath(imageID, mediaType, path string) {
	runtimeImagePathCache.Store(imagePathCacheKey(imageID, mediaType), path)
}

func invalidateRuntimeImagePaths(imageID string) {
	prefix := imageID + "\x00"
	runtimeImagePathCache.Range(func(key, _ any) bool {
		if name, ok := key.(string); ok && len(name) >= len(prefix) && name[:len(prefix)] == prefix {
			runtimeImagePathCache.Delete(key)
		}
		return true
	})
}

// openImageFile はpresetまたは実行時キャッシュの画像ファイルを開く。
// exportのようにDBの画像データ読み出しを避けたい用途で使う。
func (h *HandlerContext) openImageFile(imageID, mediaType string) (*os.File, bool) {
	if path, ok := h.cachedPresetImagePath(imageID, mediaType); ok {
		if file, err := os.Open(path); err == nil {
			return file, true
		}
	}
	if path, ok := h.cachedImagePath(imageID, mediaType); ok {
		if file, err := os.Open(path); err == nil {
			return file, true
		}
	}
	return nil, false
}

// writeImageFile は同一ディレクトリ内の一時ファイルに書いてから rename する。
// nginx が書きかけのファイルを読むことはない。ディレクトリは呼び出し側で作成しておく。
func writeImageFile(path string, data []byte) error {
	dir := filepath.Dir(path)
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

func writeImageCacheFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return writeImageFile(path, data)
}

func serveImageFile(c *echo.Context, urlPrefix, root, path, mediaType string) error {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		rel = filepath.Base(path)
	}
	c.Response().Header().Set("Content-Type", mediaType)
	c.Response().Header().Set("X-Accel-Redirect", urlPrefix+filepath.ToSlash(rel))
	return c.NoContent(http.StatusOK)
}

func (h *HandlerContext) serveCachedImage(c *echo.Context, path, mediaType string) error {
	return serveImageFile(c, imageCacheURLPrefix, h.Config.ImageCacheDir, path, mediaType)
}

func (h *HandlerContext) servePresetImage(c *echo.Context, path, mediaType string) error {
	return serveImageFile(c, presetImageURLPrefix, h.Config.PresetImageDir, path, mediaType)
}

// removeCachedImage は削除された画像のキャッシュを掃除する。失敗してもリクエストは継続する。
func (h *HandlerContext) removeCachedImage(imageID string) {
	key := imageCacheKey(imageID)
	matches, err := filepath.Glob(filepath.Join(h.Config.ImageCacheDir, key[:2], key+".*"))
	if err != nil {
		return
	}
	for _, path := range matches {
		_ = os.Remove(path)
	}
	invalidateRuntimeImagePaths(imageID)
}

// clearRuntimeImageCache は実行時キャッシュだけを消す。preset は消さない。
func (h *HandlerContext) clearRuntimeImageCache() error {
	if h.Config.ImageCacheDir == "" {
		return nil
	}
	if err := os.RemoveAll(h.Config.ImageCacheDir); err != nil {
		return err
	}
	runtimeImagePathCache.Range(func(key, _ any) bool {
		runtimeImagePathCache.Delete(key)
		return true
	})
	return os.MkdirAll(h.Config.ImageCacheDir, 0o755)
}
