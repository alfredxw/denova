package middleware

import "fmt"

func eventDataString(data interface{}, key string) string {
	switch typed := data.(type) {
	case map[string]string:
		return typed[key]
	case map[string]interface{}:
		if value, ok := typed[key]; ok {
			return fmt.Sprint(value)
		}
	}
	return ""
}

func cloneEventDataMap(data interface{}) map[string]interface{} {
	next := map[string]interface{}{}
	if typed, ok := data.(map[string]interface{}); ok {
		for key, value := range typed {
			next[key] = value
		}
		return next
	}
	if typed, ok := data.(map[string]string); ok {
		for key, value := range typed {
			next[key] = value
		}
	}
	return next
}
