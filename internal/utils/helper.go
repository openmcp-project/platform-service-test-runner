package utils

import (
	"encoding/json"
)

// GetAsString safely retrieves a string value from a map, returning an empty string if the key doesn't exist or if the value is not a string
func GetAsString(data map[string]interface{}, key string) string {
	if val, exists := data[key]; exists {
		if strVal, ok := val.(string); ok {
			return strVal
		}
	}
	return ""
}

// MarshalToRawMessage converts a map[string]interface{} to json.RawMessage
func MarshalToRawMessage(data map[string]interface{}) (json.RawMessage, error) {
	if data == nil {
		return nil, nil
	}
	raw, err := json.Marshal(data)
	if err != nil {
		// Log the error but return nil rather than panicking
		return nil, err
	}
	return raw, nil
}
