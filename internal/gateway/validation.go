package gateway

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"strings"
)

// ValidateRequestBody validates a JSON request body against a JSON Schema subset.
// Supports: type, required, properties, minLength, maxLength, minimum, maximum, pattern, enum.
func ValidateRequestBody(schemaRaw json.RawMessage, body json.RawMessage) error {
	if len(schemaRaw) == 0 {
		return nil
	}

	var schema map[string]any
	if err := json.Unmarshal(schemaRaw, &schema); err != nil {
		return fmt.Errorf("invalid schema: %w", err)
	}

	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		return fmt.Errorf("invalid JSON body: %w", err)
	}

	return validateValue("", schema, value)
}

func validateValue(path string, schema map[string]any, value any) error {
	if path == "" {
		path = "$"
	}

	// Type check
	if schemaType, ok := schema["type"].(string); ok {
		if err := checkType(path, schemaType, value); err != nil {
			return err
		}
	}

	// Enum check
	if enumRaw, ok := schema["enum"]; ok {
		if enumSlice, ok := enumRaw.([]any); ok {
			if err := checkEnum(path, enumSlice, value); err != nil {
				return err
			}
		}
	}

	switch v := value.(type) {
	case string:
		if err := validateString(path, schema, v); err != nil {
			return err
		}
	case float64:
		if err := validateNumber(path, schema, v); err != nil {
			return err
		}
	case map[string]any:
		if err := validateObject(path, schema, v); err != nil {
			return err
		}
	case []any:
		if err := validateArray(path, schema, v); err != nil {
			return err
		}
	}

	return nil
}

func checkType(path, expected string, value any) error {
	var actual string
	switch value.(type) {
	case map[string]any:
		actual = "object"
	case []any:
		actual = "array"
	case string:
		actual = "string"
	case float64:
		// JSON numbers are float64; check if it's an integer
		if expected == "integer" {
			v := value.(float64)
			if v != math.Floor(v) {
				return fmt.Errorf("%s: expected integer, got float", path)
			}
			return nil
		}
		actual = "number"
	case bool:
		actual = "boolean"
	case nil:
		actual = "null"
	default:
		actual = reflect.TypeOf(value).String()
	}

	if expected == "integer" && actual == "number" {
		return nil // integers are numbers
	}
	if actual != expected {
		return fmt.Errorf("%s: expected type %s, got %s", path, expected, actual)
	}
	return nil
}

func checkEnum(path string, allowed []any, value any) error {
	for _, a := range allowed {
		if fmt.Sprintf("%v", a) == fmt.Sprintf("%v", value) {
			return nil
		}
	}
	return fmt.Errorf("%s: value not in allowed enum values", path)
}

func validateString(path string, schema map[string]any, value string) error {
	if minLen, ok := schema["minLength"].(float64); ok {
		if len(value) < int(minLen) {
			return fmt.Errorf("%s: string length %d below minimum %d", path, len(value), int(minLen))
		}
	}
	if maxLen, ok := schema["maxLength"].(float64); ok {
		if len(value) > int(maxLen) {
			return fmt.Errorf("%s: string length %d exceeds maximum %d", path, len(value), int(maxLen))
		}
	}
	if pattern, ok := schema["pattern"].(string); ok {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return fmt.Errorf("%s: invalid pattern %q: %w", path, pattern, err)
		}
		if !re.MatchString(value) {
			return fmt.Errorf("%s: string does not match pattern %q", path, pattern)
		}
	}
	return nil
}

func validateNumber(path string, schema map[string]any, value float64) error {
	if min, ok := schema["minimum"].(float64); ok {
		if value < min {
			return fmt.Errorf("%s: value %v below minimum %v", path, value, min)
		}
	}
	if max, ok := schema["maximum"].(float64); ok {
		if value > max {
			return fmt.Errorf("%s: value %v exceeds maximum %v", path, value, max)
		}
	}
	return nil
}

func validateObject(path string, schema map[string]any, obj map[string]any) error {
	// Required fields
	if reqRaw, ok := schema["required"]; ok {
		if reqSlice, ok := reqRaw.([]any); ok {
			for _, r := range reqSlice {
				fieldName, ok := r.(string)
				if !ok {
					continue
				}
				if _, exists := obj[fieldName]; !exists {
					return fmt.Errorf("%s: missing required field %q", path, fieldName)
				}
			}
		}
	}

	// Properties
	if propsRaw, ok := schema["properties"]; ok {
		if props, ok := propsRaw.(map[string]any); ok {
			for fieldName, fieldSchemaRaw := range props {
				fieldSchema, ok := fieldSchemaRaw.(map[string]any)
				if !ok {
					continue
				}
				fieldValue, exists := obj[fieldName]
				if !exists {
					continue // not required, not present -> skip
				}
				fieldPath := path + "." + fieldName
				if err := validateValue(fieldPath, fieldSchema, fieldValue); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func validateArray(path string, schema map[string]any, arr []any) error {
	if minItems, ok := schema["minItems"].(float64); ok {
		if len(arr) < int(minItems) {
			return fmt.Errorf("%s: array length %d below minimum %d", path, len(arr), int(minItems))
		}
	}
	if maxItems, ok := schema["maxItems"].(float64); ok {
		if len(arr) > int(maxItems) {
			return fmt.Errorf("%s: array length %d exceeds maximum %d", path, len(arr), int(maxItems))
		}
	}

	// Validate items against "items" schema
	if itemsRaw, ok := schema["items"]; ok {
		if itemsSchema, ok := itemsRaw.(map[string]any); ok {
			for i, item := range arr {
				itemPath := fmt.Sprintf("%s[%d]", path, i)
				if err := validateValue(itemPath, itemsSchema, item); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// FormatValidationError creates a user-friendly error message
func FormatValidationError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	// Strip the "$." prefix for readability
	msg = strings.TrimPrefix(msg, "$.")
	return msg
}
