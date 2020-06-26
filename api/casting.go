package api

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
