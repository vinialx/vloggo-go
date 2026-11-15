package services

import (
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"time"
	"unicode"

	"github.com/vinialx/vloggo-go/v2/types"
)

const (
	redactedValue = "[REDACTED]"
	circularValue = "[Circular]"
	maxValueDepth = 20
)

type visitKey struct {
	kind reflect.Kind
	ptr  uintptr
}

// Sanitize copies a value into JSON-safe data and redacts matching field names or paths.
func Sanitize(value any, patterns []string) any {
	state := sanitizer{patterns: patterns, ancestors: make(map[visitKey]bool)}
	return state.value(reflect.ValueOf(value), nil, 0)
}

// SanitizeData converts a value into an independent structured object.
func SanitizeData(value any, patterns []string) types.LogData {
	if value == nil {
		return nil
	}
	sanitized := Sanitize(value, patterns)
	if result, ok := sanitized.(map[string]any); ok {
		return types.LogData(result)
	}
	return types.LogData{"value": sanitized}
}

// SerializeError captures available error data without requiring errors to expose a stack.
func SerializeError(err error, patterns []string) types.LogError {
	if isNilError(err) {
		return nil
	}
	state := sanitizer{patterns: patterns, ancestors: make(map[visitKey]bool)}
	return state.errorValue(err, []string{"error"}, 0)
}

type sanitizer struct {
	patterns  []string
	ancestors map[visitKey]bool
}

func (s *sanitizer) value(value reflect.Value, path []string, depth int) any {
	if !value.IsValid() {
		return nil
	}
	if depth >= maxValueDepth {
		return "[MaxDepth]"
	}

	for value.Kind() == reflect.Interface {
		if value.IsNil() {
			return nil
		}
		value = value.Elem()
	}
	if value.CanInterface() {
		if value.Type() == reflect.TypeOf(time.Time{}) {
			return value.Interface().(time.Time).Format(time.RFC3339Nano)
		}
		if err, ok := value.Interface().(error); ok {
			if isNilError(err) {
				return nil
			}
			return s.errorValue(err, path, depth)
		}
	}

	switch value.Kind() {
	case reflect.Bool:
		return value.Bool()
	case reflect.String:
		return value.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return value.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return value.Uint()
	case reflect.Float32, reflect.Float64:
		number := value.Float()
		if math.IsNaN(number) || math.IsInf(number, 0) {
			return fmt.Sprint(number)
		}
		return number
	case reflect.Ptr:
		if value.IsNil() {
			return nil
		}
		key := visitKey{kind: value.Kind(), ptr: value.Pointer()}
		return s.withAncestor(key, func() any { return s.value(value.Elem(), path, depth+1) })
	case reflect.Map:
		if value.IsNil() {
			return nil
		}
		key := visitKey{kind: value.Kind(), ptr: uintptr(value.UnsafePointer())}
		return s.withAncestor(key, func() any {
			result := make(map[string]any, value.Len())
			iterator := value.MapRange()
			for iterator.Next() {
				field := fmt.Sprint(iterator.Key().Interface())
				if s.matches(path, field) {
					result[field] = redactedValue
				} else {
					result[field] = s.value(iterator.Value(), appendPath(path, field), depth+1)
				}
			}
			return result
		})
	case reflect.Slice:
		if value.IsNil() {
			return nil
		}
		if value.Type().Elem().Kind() == reflect.Uint8 {
			bytes := make([]byte, value.Len())
			for index := range bytes {
				bytes[index] = byte(value.Index(index).Uint())
			}
			return base64.StdEncoding.EncodeToString(bytes)
		}
		key := visitKey{kind: value.Kind(), ptr: value.Pointer()}
		return s.withAncestor(key, func() any { return s.sequence(value, path, depth) })
	case reflect.Array:
		if value.Type().Elem().Kind() == reflect.Uint8 {
			bytes := make([]byte, value.Len())
			for index := range bytes {
				bytes[index] = byte(value.Index(index).Uint())
			}
			return base64.StdEncoding.EncodeToString(bytes)
		}
		return s.sequence(value, path, depth)
	case reflect.Struct:
		result := make(map[string]any)
		typeOfValue := value.Type()
		for index := 0; index < value.NumField(); index++ {
			fieldType := typeOfValue.Field(index)
			if fieldType.PkgPath != "" {
				continue
			}
			name := fieldType.Name
			if tag := fieldType.Tag.Get("json"); tag != "" {
				tagName := strings.Split(tag, ",")[0]
				if tagName == "-" {
					continue
				}
				if tagName != "" {
					name = tagName
				}
			}
			field := value.Field(index)
			if s.matches(path, name) {
				result[name] = redactedValue
			} else if field.CanInterface() {
				result[name] = s.value(field, appendPath(path, name), depth+1)
			}
		}
		return result
	case reflect.Invalid:
		return nil
	case reflect.Func:
		return "[Function]"
	case reflect.Chan, reflect.UnsafePointer:
		return fmt.Sprintf("[%s]", value.Kind())
	case reflect.Complex64, reflect.Complex128:
		return fmt.Sprint(value.Complex())
	default:
		if value.CanInterface() {
			return fmt.Sprint(value.Interface())
		}
		return fmt.Sprintf("[%s]", value.Kind())
	}
}

func (s *sanitizer) sequence(value reflect.Value, path []string, depth int) []any {
	result := make([]any, value.Len())
	for index := 0; index < value.Len(); index++ {
		key := fmt.Sprint(index)
		if s.matches(path, key) {
			result[index] = redactedValue
		} else {
			result[index] = s.value(value.Index(index), appendPath(path, key), depth+1)
		}
	}
	return result
}

func (s *sanitizer) errorValue(err error, path []string, depth int) types.LogError {
	if isNilError(err) {
		return nil
	}
	if depth >= maxValueDepth {
		return types.LogError{"name": errorName(err), "message": "[MaxDepth]"}
	}
	value := reflect.ValueOf(err)
	if value.Kind() == reflect.Ptr && !value.IsNil() {
		key := visitKey{kind: value.Kind(), ptr: value.Pointer()}
		if s.ancestors[key] {
			return types.LogError{"name": errorName(err), "message": circularValue}
		}
		s.ancestors[key] = true
		defer delete(s.ancestors, key)
	}
	result := types.LogError{
		"name":    errorName(err),
		"message": err.Error(),
	}
	if s.matches(path, "message") {
		result["message"] = redactedValue
	}
	if stack, ok := err.(interface{ StackTrace() string }); ok {
		stackText := stack.StackTrace()
		if s.matches(path, "message") {
			firstLine := errorName(err) + ": " + redactedValue
			if newline := strings.IndexByte(stackText, '\n'); newline >= 0 {
				stackText = firstLine + stackText[newline:]
			} else if stackText != "" {
				stackText = firstLine
			}
		}
		result["stack"] = stackText
	}
	if cause := errors.Unwrap(err); cause != nil {
		if s.matches(path, "cause") {
			result["cause"] = redactedValue
		} else {
			result["cause"] = s.errorValue(cause, appendPath(path, "cause"), depth+1)
		}
	}
	if causes, ok := err.(interface{ Unwrap() []error }); ok {
		items := causes.Unwrap()
		if len(items) > 0 {
			serialized := make([]any, 0, len(items))
			for _, cause := range items {
				serialized = append(serialized, s.errorValue(cause, appendPath(path, "cause"), depth+1))
			}
			result["causes"] = serialized
		}
	}
	if fields := errorFields(err); len(fields) > 0 {
		for name, value := range fields {
			if strings.EqualFold(name, "name") || strings.EqualFold(name, "message") || strings.EqualFold(name, "stack") || strings.EqualFold(name, "cause") {
				continue
			}
			if s.matches(path, name) {
				result[name] = redactedValue
			} else {
				result[name] = s.value(reflect.ValueOf(value), appendPath(path, name), depth+1)
			}
		}
	}
	return result
}

func isNilError(err error) bool {
	if err == nil {
		return true
	}
	value := reflect.ValueOf(err)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func errorName(err error) string {
	typeName := reflect.TypeOf(err).String()
	typeName = strings.TrimPrefix(typeName, "*")
	if separator := strings.LastIndex(typeName, "."); separator >= 0 {
		typeName = typeName[separator+1:]
	}
	if strings.Contains(typeName, "errorString") || strings.Contains(typeName, "wrapError") {
		return "Error"
	}
	return typeName
}

func errorFields(err error) map[string]any {
	value := reflect.ValueOf(err)
	for value.Kind() == reflect.Ptr || value.Kind() == reflect.Interface {
		if value.IsNil() {
			return nil
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return nil
	}
	fields := make(map[string]any)
	for index := 0; index < value.NumField(); index++ {
		fieldType := value.Type().Field(index)
		if fieldType.PkgPath != "" || !unicode.IsUpper(rune(fieldType.Name[0])) {
			continue
		}
		field := value.Field(index)
		if field.CanInterface() {
			fields[fieldType.Name] = field.Interface()
		}
	}
	return fields
}

func (s *sanitizer) withAncestor(key visitKey, visit func() any) any {
	if key.ptr == 0 {
		return visit()
	}
	if s.ancestors[key] {
		return circularValue
	}
	s.ancestors[key] = true
	result := visit()
	delete(s.ancestors, key)
	return result
}

func (s *sanitizer) matches(path []string, key string) bool {
	full := appendPath(path, strings.ToLower(key))
	for _, pattern := range s.patterns {
		parts := strings.Split(strings.ToLower(strings.TrimSpace(pattern)), ".")
		if len(parts) > 1 && (parts[0] == "context" || parts[0] == "data" || parts[0] == "error") {
			parts = parts[1:]
		}
		if len(parts) == 1 {
			if parts[0] == full[len(full)-1] {
				return true
			}
			continue
		}
		if len(parts) > len(full) {
			continue
		}
		suffix := full[len(full)-len(parts):]
		matched := true
		for index, part := range parts {
			if part != "*" && part != suffix[index] {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func appendPath(path []string, value string) []string {
	result := make([]string, len(path)+1)
	copy(result, path)
	result[len(path)] = strings.ToLower(value)
	return result
}
