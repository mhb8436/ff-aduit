package spec

import (
	"fmt"
	"strconv"
)

// YAML(map[string]any) 값에 타입 안전하게 접근하는 헬퍼.
// yaml.v3 는 정수를 int, 실수를 float64, 목록을 []any 로 디코드한다.

// Map 은 하위 맵을 반환(없으면 빈 맵).
func Map(m map[string]any, key string) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	if v, ok := m[key].(map[string]any); ok {
		return v
	}
	return map[string]any{}
}

// Has 는 키 존재 여부.
func Has(m map[string]any, key string) bool {
	if m == nil {
		return false
	}
	_, ok := m[key]
	return ok
}

// Str 은 문자열 값(없으면 "").
func Str(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		return toStr(v)
	}
	return ""
}

// Bool 은 불리언 값(없으면 def).
func Bool(m map[string]any, key string, def bool) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return def
}

// Float 은 실수 값(없거나 변환 실패 시 def). 키가 없으면 hasKey=false.
func Float(m map[string]any, key string, def float64) (val float64, has bool) {
	v, ok := m[key]
	if !ok || v == nil {
		return def, false
	}
	return toFloat(v, def), true
}

// Int 는 정수 값(없으면 def).
func Int(m map[string]any, key string, def int) int {
	v, ok := m[key]
	if !ok {
		return def
	}
	return int(toFloat(v, float64(def)))
}

// StrList 는 문자열 목록.
func StrList(m map[string]any, key string) []string {
	arr, ok := m[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, e := range arr {
		out = append(out, toStr(e))
	}
	return out
}

// FloatList 는 실수 목록.
func FloatList(m map[string]any, key string) []float64 {
	arr, ok := m[key].([]any)
	if !ok {
		return nil
	}
	out := make([]float64, 0, len(arr))
	for _, e := range arr {
		out = append(out, toFloat(e, 0))
	}
	return out
}

// IntList 는 정수 목록.
func IntList(m map[string]any, key string) []int {
	arr, ok := m[key].([]any)
	if !ok {
		return nil
	}
	out := make([]int, 0, len(arr))
	for _, e := range arr {
		out = append(out, int(toFloat(e, 0)))
	}
	return out
}

func toStr(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case fmt.Stringer:
		return t.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}

func toFloat(v any, def float64) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case float32:
		return float64(t)
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case string:
		if f, err := strconv.ParseFloat(t, 64); err == nil {
			return f
		}
	}
	return def
}
