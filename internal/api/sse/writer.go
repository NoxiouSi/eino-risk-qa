package sse

import (
	"encoding/json"
	"fmt"
	"io"
)

// WriteEvent 按标准 SSE 文本协议写出一个事件帧：
//
//	event: <event>
//	data: <json(payload)>
//	\n
//
// payload 会被序列化为单行 JSON（SSE data 字段不允许原始换行）。
func WriteEvent(w io.Writer, event string, payload interface{}) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
	return err
}
