package isureal

import (
	"bytes"
	"errors"
	"image/jpeg"
	"io"
)

var errImageTooLarge = errors.New("image too large")

// readJPEGWithConfig は JPEG の寸法を先に確認し、上限を超える画像は本文の残りを
// 読まずに拒否する。寸法が範囲内の場合は最後まで読むが、JPEG本体の完全デコードは
// 行わない。投稿・RealMojiのように正常画像だけを受け付ける用途向け。
func readJPEGWithConfig(r io.Reader, maxDimension int) ([]byte, error) {
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
	return buf.Bytes(), nil
}

// readValidatedJPEG は寸法確認に加えて jpeg.Decode でJPEG本体の完全性も検証する。
// プロフィール画像のように不正画像を厳密に弾きたい用途向け。
func readValidatedJPEG(r io.Reader, maxDimension int) ([]byte, error) {
	data, err := readJPEGWithConfig(r, maxDimension)
	if err != nil {
		return nil, err
	}
	if _, err := jpeg.Decode(bytes.NewReader(data)); err != nil {
		return nil, err
	}
	return data, nil
}
