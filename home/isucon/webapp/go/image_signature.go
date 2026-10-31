package isureal

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"sync"
	"time"
)

const imageURLLifetime = 24 * time.Hour
const imageURLExpiryBucket = 5 * time.Second
const imageSignatureIterations = 1000
const imageSignatureCacheTTL = time.Minute

type imageSignatureCacheEntry struct {
	signature []byte
	expiresAt time.Time
}

var imageSignatureCache sync.Map
var imageSignatureCacheCleanupOnce sync.Once

func startImageSignatureCacheCleanup() {
	imageSignatureCacheCleanupOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for now := range ticker.C {
				imageSignatureCache.Range(func(key, value any) bool {
					entry, ok := value.(imageSignatureCacheEntry)
					if !ok || !now.Before(entry.expiresAt) {
						imageSignatureCache.Delete(key)
					}
					return true
				})
			}
		}()
	})
}

var imageSigningKeyCache sync.Map

func imageSigningKey(secretKeyHash string) ([]byte, error) {
	if value, ok := imageSigningKeyCache.Load(secretKeyHash); ok {
		return value.([]byte), nil
	}
	key, err := hex.DecodeString(secretKeyHash)
	if err != nil || len(key) != sha256.Size {
		return nil, fmt.Errorf("invalid image signing key")
	}
	imageSigningKeyCache.Store(secretKeyHash, key)
	return key, nil
}

func computeImageSignature(key []byte, imageID string, exp int64) []byte {
	// HMACオブジェクトと作業バッファを再利用する。
	// 1000回の反復ごとにhmac.Newでキースケジュールを作り直すとCPU負荷が高い。
	value := make([]byte, 0, 64)
	value = strconv.AppendInt(value, exp, 10)
	value = append(value, ':')
	value = append(value, imageID...)

	mac := hmac.New(sha256.New, key)
	for range imageSignatureIterations {
		mac.Reset()
		mac.Write(value)
		value = mac.Sum(value[:0])
	}
	return value
}

func imageSignature(key []byte, imageID string, exp int64) []byte {
	cacheKey := string(key) + "\x00" + imageID + "\x00" + strconv.FormatInt(exp, 10)
	now := time.Now()
	if value, ok := imageSignatureCache.Load(cacheKey); ok {
		entry, ok := value.(imageSignatureCacheEntry)
		if ok && now.Before(entry.expiresAt) {
			return append([]byte(nil), entry.signature...)
		}
		imageSignatureCache.Delete(cacheKey)
	}

	signature := computeImageSignature(key, imageID, exp)
	imageSignatureCache.Store(cacheKey, imageSignatureCacheEntry{
		signature: append([]byte(nil), signature...),
		expiresAt: now.Add(imageSignatureCacheTTL),
	})
	return signature
}

func signedImageURL(user User, imageID string) string {
	// 5秒間は同じexp・同じ署名を返す。署名キャッシュのヒット率を上げる。
	exp := time.Now().Add(imageURLLifetime).Truncate(imageURLExpiryBucket).Unix()
	query := url.Values{
		"exp": {strconv.FormatInt(exp, 10)},
		"sig": {base64.RawURLEncoding.EncodeToString(imageSignature(user.ImageKey, imageID, exp))},
	}
	return "/api/images/" + url.PathEscape(imageID) + "?" + query.Encode()
}
