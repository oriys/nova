package domain

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"unicode"
)

// ApplyParamMappings extracts values from the HTTP request according to the
// ParamMapping rules and merges them into the function payload.
// It supports query parameters, path parameters, request body fields, and
// HTTP headers with optional case transformation and type coercion.
func ApplyParamMappings(
	payload json.RawMessage,
	r *http.Request,
	pathParams map[string]string,
	mappings []ParamMapping,
) (json.RawMessage, error) {
	if len(mappings) == 0 {
		return payload, nil
	}

	var obj map[string]any
	if err := json.Unmarshal(payload, &obj); err != nil {
		obj = make(map[string]any)
	}

	var bodyObj map[string]any
	var bodyParsed bool

	for _, m := range mappings {
		target := m.Target
		if target == "" {
			target = m.Name
		}

		var raw string
		var found bool

		switch m.Source {
		case ParamSourceQuery:
			if r.URL.Query().Has(m.Name) {
				raw = r.URL.Query().Get(m.Name)
				found = true
			}

		case ParamSourcePath:
			if v, ok := pathParams[m.Name]; ok {
				raw = v
				found = true
			}

		case ParamSourceHeader:
			if v := r.Header.Get(m.Name); v != "" {
				raw = v
				found = true
			}

		case ParamSourceBody:
			if !bodyParsed {
				bodyParsed = true
				_ = json.Unmarshal(payload, &bodyObj)
			}
			if bodyObj != nil {
				if v, ok := bodyObj[m.Name]; ok {
					converted, err := CoerceBodyValue(v, m.Type, m.Transform)
					if err != nil {
						return nil, fmt.Errorf("param %q: %w", m.Name, err)
					}
					obj[target] = converted
					continue
				}
			}
		}

		if !found {
			if m.Required {
				return nil, fmt.Errorf("required parameter %q missing from %s", m.Name, m.Source)
			}
			if m.Default != nil {
				obj[target] = m.Default
			}
			continue
		}

		raw = TransformCase(raw, m.Transform)

		val, err := CoerceString(raw, m.Type)
		if err != nil {
			return nil, fmt.Errorf("param %q type coercion (%s): %w", m.Name, m.Type, err)
		}
		obj[target] = val
	}

	out, err := json.Marshal(obj)
	if err != nil {
		return payload, nil
	}
	return out, nil
}

// CoerceString converts a string value to the target type.
func CoerceString(s string, t ParamType) (any, error) {
	switch t {
	case ParamTypeString:
		return s, nil
	case ParamTypeInteger:
		return strconv.ParseInt(s, 10, 64)
	case ParamTypeFloat:
		return strconv.ParseFloat(s, 64)
	case ParamTypeBoolean:
		return ParseBool(s)
	case ParamTypeJSON:
		var v any
		if err := json.Unmarshal([]byte(s), &v); err != nil {
			return nil, fmt.Errorf("invalid JSON: %w", err)
		}
		return v, nil
	default:
		return s, nil
	}
}

// CoerceBodyValue converts a value already parsed from JSON to the target type.
func CoerceBodyValue(v any, t ParamType, transform ParamTransform) (any, error) {
	if s, ok := v.(string); ok && transform != "" {
		v = TransformCase(s, transform)
	}

	switch t {
	case ParamTypeString:
		switch val := v.(type) {
		case string:
			return TransformCase(val, transform), nil
		case float64:
			if val == float64(int64(val)) {
				return strconv.FormatInt(int64(val), 10), nil
			}
			return strconv.FormatFloat(val, 'f', -1, 64), nil
		case bool:
			return strconv.FormatBool(val), nil
		default:
			b, _ := json.Marshal(v)
			return string(b), nil
		}
	case ParamTypeInteger:
		switch val := v.(type) {
		case float64:
			return int64(val), nil
		case string:
			return strconv.ParseInt(val, 10, 64)
		case bool:
			if val {
				return int64(1), nil
			}
			return int64(0), nil
		default:
			return nil, fmt.Errorf("cannot convert %T to integer", v)
		}
	case ParamTypeFloat:
		switch val := v.(type) {
		case float64:
			return val, nil
		case string:
			return strconv.ParseFloat(val, 64)
		default:
			return nil, fmt.Errorf("cannot convert %T to float", v)
		}
	case ParamTypeBoolean:
		switch val := v.(type) {
		case bool:
			return val, nil
		case float64:
			return val != 0, nil
		case string:
			return ParseBool(val)
		default:
			return nil, fmt.Errorf("cannot convert %T to boolean", v)
		}
	case ParamTypeJSON:
		return v, nil
	}
	return v, nil
}

// ParseBool extends strconv.ParseBool with extra truthy/falsy values.
func ParseBool(s string) (bool, error) {
	switch strings.ToLower(s) {
	case "true", "1", "yes", "on":
		return true, nil
	case "false", "0", "no", "off", "":
		return false, nil
	default:
		return false, fmt.Errorf("cannot parse %q as boolean", s)
	}
}

// TransformCase applies the given case transformation to a string value.
func TransformCase(s string, t ParamTransform) string {
	switch t {
	case ParamTransformUpperCase:
		return strings.ToUpper(s)
	case ParamTransformLowerCase:
		return strings.ToLower(s)
	case ParamTransformUpperFirst:
		if s == "" {
			return s
		}
		r := []rune(s)
		r[0] = unicode.ToUpper(r[0])
		return string(r)
	case ParamTransformCamelCase:
		return toCamelCase(s)
	case ParamTransformSnakeCase:
		return toSnakeCase(s)
	case ParamTransformKebabCase:
		return toKebabCase(s)
	default:
		return s
	}
}

func toCamelCase(s string) string {
	parts := splitIdentifier(s)
	if len(parts) == 0 {
		return s
	}
	var b strings.Builder
	for i, p := range parts {
		if p == "" {
			continue
		}
		if i == 0 {
			b.WriteString(strings.ToLower(p))
		} else {
			r := []rune(strings.ToLower(p))
			r[0] = unicode.ToUpper(r[0])
			b.WriteString(string(r))
		}
	}
	return b.String()
}

func toSnakeCase(s string) string {
	return joinIdentifier(splitCamel(s), '_')
}

func toKebabCase(s string) string {
	return joinIdentifier(splitCamel(s), '-')
}

func splitIdentifier(s string) []string {
	if strings.Contains(s, "_") {
		return strings.Split(s, "_")
	}
	if strings.Contains(s, "-") {
		return strings.Split(s, "-")
	}
	return splitCamel(s)
}

func splitCamel(s string) []string {
	var parts []string
	runes := []rune(s)
	start := 0
	for i := 1; i < len(runes); i++ {
		if unicode.IsUpper(runes[i]) && (i+1 >= len(runes) || unicode.IsLower(runes[i+1]) || unicode.IsLower(runes[i-1])) {
			parts = append(parts, string(runes[start:i]))
			start = i
		}
	}
	parts = append(parts, string(runes[start:]))
	return parts
}

func joinIdentifier(parts []string, sep rune) string {
	var b strings.Builder
	for i, p := range parts {
		if p == "" {
			continue
		}
		if i > 0 {
			b.WriteRune(sep)
		}
		b.WriteString(strings.ToLower(p))
	}
	return b.String()
}
