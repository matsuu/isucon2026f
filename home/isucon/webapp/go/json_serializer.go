package isureal

import (
	"io"

	json "github.com/goccy/go-json"
	"github.com/labstack/echo/v5"
)

// JSONSerializer はEchoのレスポンス/リクエストJSONをgoccy/go-jsonで処理する。
type JSONSerializer struct{}

func (JSONSerializer) Serialize(c *echo.Context, target any, indent string) error {
	var (
		data []byte
		err  error
	)
	if indent != "" {
		data, err = json.MarshalIndent(target, "", indent)
	} else {
		data, err = json.Marshal(target)
	}
	if err != nil {
		return err
	}
	// encoding/json.Encoder.Encode と同じく末尾に改行を付ける。
	data = append(data, '\n')
	_, err = c.Response().Write(data)
	return err
}

func (JSONSerializer) Deserialize(c *echo.Context, target any) error {
	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return echo.ErrBadRequest.Wrap(err)
	}
	if err := json.Unmarshal(body, target); err != nil {
		return echo.ErrBadRequest.Wrap(err)
	}
	return nil
}
