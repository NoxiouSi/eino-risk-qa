package persistence

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// JSONSlice 用于持久化任意JSON数组快照。
type JSONSlice []map[string]interface{}

func (j JSONSlice) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

func (j *JSONSlice) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	var data []byte
	switch v := value.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return fmt.Errorf("unsupported JSONSlice scan type %T", value)
	}
	return json.Unmarshal(data, j)
}
