package appconfig

import (
	"fmt"
	"reflect"
	"strings"

	io "github.com/superfly/flyctl/iostreams"
)

// StrictValidateResult contains the results of strict validation
type StrictValidateResult struct {
	UnrecognizedSections []string
	UnrecognizedKeys     map[string][]string // section -> keys
}

// StrictValidate performs strict validation on a raw configuration map
// by checking for unrecognized sections and keys using reflection on the Config type
func StrictValidate(rawConfig map[string]any) *StrictValidateResult {
	result := &StrictValidateResult{
		UnrecognizedSections: []string{},
		UnrecognizedKeys:     make(map[string][]string),
	}

	recognizedFields := getFields(reflect.TypeOf(Config{}))

	// Check each key in the raw config
	for key, value := range rawConfig {
		fieldInfo, recognized := recognizedFields[key]
		if !recognized {
			result.UnrecognizedSections = append(result.UnrecognizedSections, key)
			continue
		}

		// If this is a map or section, check its nested keys
		if fieldInfo.isNested && value != nil {
			validateNestedSection(key, value, fieldInfo.fieldType, result)
		}
	}

	return result
}

// fieldInfo stores information about a struct field
type fieldInfo struct {
	fieldType reflect.Type
	isNested  bool
}

// getFields extracts all recognized field names from struct tags
func getFields(t reflect.Type) map[string]fieldInfo {
	fields := make(map[string]fieldInfo)

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		// Check for toml tag first, then json tag
		tomlTag := field.Tag.Get("toml")
		jsonTag := field.Tag.Get("json")

		// Skip fields marked with "-"
		if tomlTag == "-" || jsonTag == "-" {
			continue
		}

		// Parse tag to get field name
		var fieldName string
		if tomlTag != "" {
			fieldName = strings.Split(tomlTag, ",")[0]
		} else if jsonTag != "" {
			fieldName = strings.Split(jsonTag, ",")[0]
		} else {
			// Use field name if no tags
			fieldName = strings.ToLower(field.Name)
		}

		if fieldName == "" || fieldName == "-" {
			continue
		}

		// Determine if this is a nested type that needs further validation
		fieldType := field.Type

		// Dereference pointers
		if fieldType.Kind() == reflect.Ptr {
			fieldType = fieldType.Elem()
		}

		isNested := isNestedType(fieldType)

		fields[fieldName] = fieldInfo{
			fieldType: fieldType,
			isNested:  isNested,
		}
	}

	return fields
}

func isNestedType(t reflect.Type) bool {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if t.Kind() == reflect.Struct &&
		!isBuiltinType(t) {
		return true
	}

	// Check if it's a slice of structs
	if t.Kind() == reflect.Slice {
		elemType := t.Elem()
		if elemType.Kind() == reflect.Ptr {
			elemType = elemType.Elem()
		}
		if elemType.Kind() == reflect.Struct && !isBuiltinType(elemType) {
			return true
		}
	}

	if t.Kind() == reflect.Map {
		return isNestedType(t.Elem())
	}

	return false
}

// isBuiltinType checks if a type is a builtin that shouldn't be recursively validated
func isBuiltinType(t reflect.Type) bool {
	pkg := t.PkgPath()
	return (pkg == "" || strings.HasPrefix(pkg, "time"))
}

// validateNestedSection validates keys within a nested section
func validateNestedSection(sectionName string, value any, expectedType reflect.Type, result *StrictValidateResult) {
	if valueMap, ok := value.(map[string]any); ok {
		// Dereference pointer types
		if expectedType.Kind() == reflect.Ptr {
			expectedType = expectedType.Elem()
		}

		// For regular structs, validate against struct fields
		if expectedType.Kind() == reflect.Struct {
			validateStructKeys(sectionName, valueMap, expectedType, result)
			return

		}

		// For maps, validate each key if it's a nested type
		if expectedType.Kind() == reflect.Map && isNestedType(expectedType.Elem()) {
			subType := expectedType.Elem()
			if subType.Kind() == reflect.Ptr {
				subType = subType.Elem()
			}

			for key, value := range valueMap {
				section := fmt.Sprintf("%s.%s", sectionName, key)
				validateNestedSection(section, value, subType, result)
			}
			return
		}
	}

	// For slices, validate each element if it's a nested type
	if valueSlice, ok := value.([]any); ok && expectedType.Kind() == reflect.Slice {
		elemType := expectedType.Elem()
		if elemType.Kind() == reflect.Ptr {
			elemType = elemType.Elem()
		}

		if isNestedType(elemType) {
			for i, elem := range valueSlice {
				section := fmt.Sprintf("%s[%d]", sectionName, i)
				validateNestedSection(section, elem, elemType, result)
			}
			return
		}
	}
}

// validateStructKeys validates that all keys in a map are recognized fields in the struct
func validateStructKeys(sectionPath string, data map[string]any, structType reflect.Type, result *StrictValidateResult) {
	recognizedFields := getFields(structType)

	// Check for inline embedded structs
	inlineFields := getInlineFields(structType)

	for key, value := range data {
		// First check regular fields
		fieldInfo, recognized := recognizedFields[key]

		// If not found in regular fields, check inline embedded fields
		if !recognized {
			for _, inlineType := range inlineFields {
				inlineRecognized := getFields(inlineType)
				if _, ok := inlineRecognized[key]; ok {
					recognized = true
					break
				}
			}
		}

		if !recognized {
			if result.UnrecognizedKeys[sectionPath] == nil {
				result.UnrecognizedKeys[sectionPath] = []string{}
			}
			result.UnrecognizedKeys[sectionPath] = append(result.UnrecognizedKeys[sectionPath], key)
			continue
		}

		// If this field is also nested, validate it recursively
		if recognized && fieldInfo.isNested && value != nil {
			nestedPath := fmt.Sprintf("%s.%s", sectionPath, key)
			validateNestedSection(nestedPath, value, fieldInfo.fieldType, result)
		}
	}
}

// getInlineFields finds all fields with inline tags
func getInlineFields(t reflect.Type) []reflect.Type {
	var inlineTypes []reflect.Type

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Check if field has inline tag
		tomlTag := field.Tag.Get("toml")
		jsonTag := field.Tag.Get("json")

		if strings.Contains(tomlTag, "inline") || strings.Contains(jsonTag, "inline") {
			fieldType := field.Type
			if fieldType.Kind() == reflect.Ptr {
				fieldType = fieldType.Elem()
			}
			inlineTypes = append(inlineTypes, fieldType)
		}
	}

	return inlineTypes
}

// FormatStrictValidationErrors formats the strict validation results as a user-friendly string
func FormatStrictValidationErrors(result *StrictValidateResult) string {
	if len(result.UnrecognizedSections) == 0 && len(result.UnrecognizedKeys) == 0 {
		return ""
	}

	var parts []string

	scheme := io.System().ColorScheme()

	if len(result.UnrecognizedSections) > 0 {
		for _, section := range result.UnrecognizedSections {
			parts = append(parts, fmt.Sprintf("  - %s", scheme.Red(section)))
		}
	}

	if len(result.UnrecognizedKeys) > 0 {
		for section, keys := range result.UnrecognizedKeys {
			for _, key := range keys {
				parts = append(parts, fmt.Sprintf("  - %s.%s", section, scheme.Red(key)))
			}
		}
	}

	return strings.Join(parts, "\n")
}
