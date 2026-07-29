package persistence

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
)

// JSONMap 是 map[string]interface{} 到 MySQL JSON 列的映射类型，
// 实现 sql.Scanner / driver.Valuer 以配合 GORM 读写 json 类型字段。
type JSONMap map[string]interface{}

// Value 实现 driver.Valuer：写入时序列化为 JSON 字符串；nil map 写入 SQL NULL。
func (m JSONMap) Value() (driver.Value, error) {
	if m == nil {
		return nil, nil
	}
	b, err := json.Marshal(map[string]interface{}(m))
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

// Scan 实现 sql.Scanner：从数据库读出的 []byte/string/nil 解析为 JSONMap。
func (m *JSONMap) Scan(value interface{}) error {
	if value == nil {
		*m = nil
		return nil
	}
	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return errors.New("persistence: unsupported Scan type for JSONMap")
	}
	if len(bytes) == 0 {
		*m = nil
		return nil
	}
	result := make(map[string]interface{})
	if err := json.Unmarshal(bytes, &result); err != nil {
		return err
	}
	*m = JSONMap(result)
	return nil
}
