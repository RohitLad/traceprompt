package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
)

// JSONMap is a map[string]any persisted as JSONB.
type JSONMap map[string]any

// Scan implements sql.Scanner.
func (m *JSONMap) Scan(value any) error {
	if value == nil {
		*m = JSONMap{}
		return nil
	}
	var b []byte
	switch v := value.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		return fmt.Errorf("unsupported JSONMap scan type %T", value)
	}
	if len(b) == 0 {
		*m = JSONMap{}
		return nil
	}
	return json.Unmarshal(b, m)
}

// Value implements driver.Valuer.
func (m JSONMap) Value() (driver.Value, error) {
	if m == nil {
		return "{}", nil
	}
	return json.Marshal(m)
}

// StringArr persists a string slice as Postgres text[].
type StringArr []string

// Scan implements sql.Scanner.
func (a *StringArr) Scan(value any) error {
	if value == nil {
		*a = StringArr{}
		return nil
	}
	switch v := value.(type) {
	case []byte:
		return a.scanPGArray(string(v))
	case string:
		return a.scanPGArray(v)
	default:
		return fmt.Errorf("unsupported StringArr scan type %T", value)
	}
}

func (a *StringArr) scanPGArray(s string) error {
	s = strings.Trim(s, "{}")
	if s == "" {
		*a = StringArr{}
		return nil
	}
	*a = strings.Split(s, ",")
	return nil
}

// Value implements driver.Valuer.
func (a StringArr) Value() (driver.Value, error) {
	if len(a) == 0 {
		return "{}", nil
	}
	quoted := make([]string, len(a))
	for i, s := range a {
		quoted[i] = `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
	}
	return "{" + strings.Join(quoted, ",") + "}", nil
}
