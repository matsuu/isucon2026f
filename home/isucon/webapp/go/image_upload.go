package isureal

import (
	"bytes"
	"errors"
	"image/jpeg"
	"io"
)

var errImageTooLarge = errors.New("image too large")

// readValidatedJPEG は JPEG の寸法を先に確認し、上限を超える画像は本文の残りを
// 読まずに拒否する。寸法が範囲内の場合は最後まで読み、jpeg.Decode で完全性も検証する。
func readValidatedJPEG(r io.Reader, maxDimension int) ([]byte, error) {
	var buf bytes.Buffer
	tee := io.TeeReader(r, &buf)

	cfg, err := jpeg.DecodeConfig(tee)
	if err != nil {
		return nil, err
	}
	if cfg.Width > maxDimension || cfg.Height > maxDimension {
		return nil, errImageTooLarge
	}
	if _, err := io.Copy(&buf, r); err != nil {
		return nil, err
	}

	data := buf.Bytes()
	if _, err := jpeg.Decode(bytes.NewReader(data)); err != nil {
		return nil, err
	}
	return data, nil
}
