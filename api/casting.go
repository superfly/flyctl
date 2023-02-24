package api

import "encoding/json"

// IntPointer - Returns a pointer to an int
func IntPointer(val int) *int {
	return &val
}

// BoolPointer - Returns a pointer to a bool
func BoolPointer(val bool) *bool {
	return &val
}

// StringPointer - Returns a pointer to a string
func StringPointer(val string) *string {
	return &val
}

// Pointer - Returns a pointer to a any type
func Pointer[T any](val T) *T {
	return &val
}

func InterfaceToMapOfStringInterface(val interface{}) (map[string]interface{}, error) {
	jsonString, err := json.Marshal(val)
	if err != nil {
		return nil, err
	}
	var outputMap map[string]interface{}
	err = json.Unmarshal(jsonString, &outputMap)
	if err != nil {
		return nil, err
	}
	return outputMap, nil
}
