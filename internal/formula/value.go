// Package formula 解析并计算考核方案中的受限表达式。
package formula

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// Kind 是表达式值的类型。
type Kind int

const (
	KindNull Kind = iota
	KindNumber
	KindString
	KindBool
	KindList
	KindObject
)

// Value 是表达式求值得到的值。
type Value struct {
	kind   Kind
	number float64
	text   string
	flag   bool
	list   []Value
	object map[string]Value
}

// Number 返回一个数值。
func Number(value float64) Value {
	return Value{kind: KindNumber, number: value}
}

// String 返回一个字符串。
func String(value string) Value {
	return Value{kind: KindString, text: value}
}

// Bool 返回一个布尔值。
func Bool(value bool) Value {
	return Value{kind: KindBool, flag: value}
}

// Null 返回空值。
func Null() Value {
	return Value{kind: KindNull}
}

// List 返回一个列表。
func List(values ...Value) Value {
	return Value{kind: KindList, list: values}
}

// Object 返回一个按键取值的对象。
func Object(values map[string]Value) Value {
	return Value{kind: KindObject, object: values}
}

// Kind 返回值的类型。
func (v Value) Kind() Kind {
	return v.kind
}

// IsNull 判断是否为空值。
func (v Value) IsNull() bool {
	return v.kind == KindNull
}

// Number 返回数值和是否为数值类型。
func (v Value) Number() (float64, bool) {
	return v.number, v.kind == KindNumber
}

// String 返回字符串和是否为字符串类型。
func (v Value) String() (string, bool) {
	return v.text, v.kind == KindString
}

// Bool 返回布尔值和是否为布尔类型。
func (v Value) Bool() (bool, bool) {
	return v.flag, v.kind == KindBool
}

// List 返回列表和是否为列表类型。
func (v Value) List() ([]Value, bool) {
	return v.list, v.kind == KindList
}

// Object 返回对象和是否为对象类型。
func (v Value) Object() (map[string]Value, bool) {
	return v.object, v.kind == KindObject
}

// Equal 判断两个值是否相等。
func (v Value) Equal(other Value) bool {
	if v.kind != other.kind {
		return false
	}
	switch v.kind {
	case KindNull:
		return true
	case KindNumber:
		return v.number == other.number
	case KindString:
		return v.text == other.text
	case KindBool:
		return v.flag == other.flag
	case KindList:
		if len(v.list) != len(other.list) {
			return false
		}
		for i := range v.list {
			if !v.list[i].Equal(other.list[i]) {
				return false
			}
		}
		return true
	case KindObject:
		if len(v.object) != len(other.object) {
			return false
		}
		for key, item := range v.object {
			otherItem, ok := other.object[key]
			if !ok || !item.Equal(otherItem) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// Format 返回用于错误信息的文本形式。
func (v Value) Format() string {
	switch v.kind {
	case KindNull:
		return "null"
	case KindNumber:
		return fmt.Sprintf("%g", v.number)
	case KindString:
		return fmt.Sprintf("%q", v.text)
	case KindBool:
		return fmt.Sprintf("%t", v.flag)
	case KindList:
		parts := make([]string, len(v.list))
		for i, item := range v.list {
			parts[i] = item.Format()
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case KindObject:
		keys := make([]string, 0, len(v.object))
		for key := range v.object {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		parts := make([]string, len(keys))
		for i, key := range keys {
			parts[i] = key + ": " + v.object[key].Format()
		}
		return "{" + strings.Join(parts, ", ") + "}"
	default:
		return "<unknown>"
	}
}

func (v Value) numeric() (float64, bool) {
	if v.kind != KindNumber {
		return 0, false
	}
	if math.IsNaN(v.number) || math.IsInf(v.number, 0) {
		return 0, false
	}
	return v.number, true
}
