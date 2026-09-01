package agent

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/nankedr/pig/ai"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func validateAndSealAgentToolArguments(tool ErasedAgentTool, value ai.JSONValue) (validatedAgentToolArguments, error) {
	schemaDocument, compiled, err := compileAgentToolSchema(tool.Name, tool.Parameters)
	if err != nil {
		return validatedAgentToolArguments{}, err
	}
	coerced := cloneAgentJSONValue(value)
	coerced = coerceAgentToolValue(coerced, schemaDocument)
	if err := compiled.Validate(coerced); err != nil {
		return validatedAgentToolArguments{}, fmt.Errorf("Validation failed for tool %q:\n  - %s", tool.Name, err)
	}
	sealed, err := sealValidatedAgentToolArguments(tool, coerced)
	if err != nil {
		return validatedAgentToolArguments{}, err
	}
	return sealed, nil
}

func compileAgentToolSchema(name string, parameters json.RawMessage) (any, *jsonschema.Schema, error) {
	schemaDocument, err := jsonschema.UnmarshalJSON(strings.NewReader(string(parameters)))
	if err != nil {
		return nil, nil, fmt.Errorf("invalid JSON Schema for tool %q: %w", name, err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft7)
	if err := compiler.AddResource("tool-schema.json", schemaDocument); err != nil {
		return nil, nil, fmt.Errorf("invalid JSON Schema for tool %q: %w", name, err)
	}
	compiled, err := compiler.Compile("tool-schema.json")
	if err != nil {
		return nil, nil, fmt.Errorf("invalid JSON Schema for tool %q: %w", name, err)
	}
	return schemaDocument, compiled, nil
}

func coerceAgentToolValue(value any, rawSchema any) any {
	schema, ok := rawSchema.(map[string]any)
	if !ok {
		return value
	}
	if allOf, ok := schema["allOf"].([]any); ok {
		for _, nested := range allOf {
			value = coerceAgentToolValue(value, nested)
		}
	}
	for _, keyword := range []string{"anyOf", "oneOf"} {
		if alternatives, ok := schema[keyword].([]any); ok {
			value = coerceAgentToolUnion(value, alternatives)
		}
	}
	types := agentSchemaTypes(schema["type"])
	if len(types) > 0 && !(len(types) > 1 && anyAgentSchemaTypeMatches(value, types)) {
		for _, schemaType := range types {
			if candidate, changed := coerceAgentToolPrimitive(value, schemaType); changed {
				value = candidate
				break
			}
		}
	}
	if object, ok := value.(map[string]any); ok && containsString(types, "object") {
		properties, _ := schema["properties"].(map[string]any)
		for name, propertySchema := range properties {
			if propertyValue, exists := object[name]; exists {
				object[name] = coerceAgentToolValue(propertyValue, propertySchema)
			}
		}
		if additional, ok := schema["additionalProperties"].(map[string]any); ok {
			for name, propertyValue := range object {
				if _, defined := properties[name]; !defined {
					object[name] = coerceAgentToolValue(propertyValue, additional)
				}
			}
		}
	}
	if array, ok := value.([]any); ok && containsString(types, "array") {
		switch items := schema["items"].(type) {
		case map[string]any:
			for i := range array {
				array[i] = coerceAgentToolValue(array[i], items)
			}
		case []any:
			for i := range array {
				if i < len(items) {
					array[i] = coerceAgentToolValue(array[i], items[i])
				}
			}
		}
	}
	return value
}

func coerceAgentToolUnion(value any, schemas []any) any {
	for _, schema := range schemas {
		validator := compileAgentSubschema(schema)
		if validator != nil && validator.Validate(value) == nil {
			return value
		}
	}
	for _, schema := range schemas {
		validator := compileAgentSubschema(schema)
		candidate := coerceAgentToolValue(cloneAgentJSONValue(value), schema)
		if validator != nil && validator.Validate(candidate) == nil {
			return candidate
		}
	}
	return value
}

func compileAgentSubschema(document any) *jsonschema.Schema {
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft7)
	if err := compiler.AddResource("subschema.json", document); err != nil {
		return nil
	}
	compiled, err := compiler.Compile("subschema.json")
	if err != nil {
		return nil
	}
	return compiled
}

func coerceAgentToolPrimitive(value any, schemaType string) (any, bool) {
	switch schemaType {
	case "number":
		return coerceAgentNumber(value, false)
	case "integer":
		return coerceAgentNumber(value, true)
	case "boolean":
		switch value := value.(type) {
		case nil:
			return false, true
		case string:
			if value == "true" {
				return true, true
			}
			if value == "false" {
				return false, true
			}
		case float64:
			if value == 1 {
				return true, true
			}
			if value == 0 {
				return false, true
			}
		case json.Number:
			if value == "1" {
				return true, true
			}
			if value == "0" {
				return false, true
			}
		}
	case "string":
		switch value := value.(type) {
		case nil:
			return "", true
		case bool:
			return strconv.FormatBool(value), true
		case float64:
			return strconv.FormatFloat(value, 'g', -1, 64), true
		case json.Number:
			return value.String(), true
		}
	case "null":
		switch value := value.(type) {
		case string:
			if value == "" {
				return nil, true
			}
		case float64:
			if value == 0 {
				return nil, true
			}
		case bool:
			if !value {
				return nil, true
			}
		}
	}
	return value, false
}

func coerceAgentNumber(value any, integer bool) (any, bool) {
	switch value := value.(type) {
	case nil:
		return float64(0), true
	case bool:
		if value {
			return float64(1), true
		}
		return float64(0), true
	case string:
		if strings.TrimSpace(value) == "" {
			return value, false
		}
		parsed, err := strconv.ParseFloat(value, 64)
		if err == nil && (!integer || math.Trunc(parsed) == parsed) {
			return parsed, true
		}
	}
	return value, false
}

func agentSchemaTypes(raw any) []string {
	switch raw := raw.(type) {
	case string:
		return []string{raw}
	case []any:
		types := make([]string, 0, len(raw))
		for _, item := range raw {
			if value, ok := item.(string); ok {
				types = append(types, value)
			}
		}
		return types
	default:
		return nil
	}
}

func anyAgentSchemaTypeMatches(value any, types []string) bool {
	for _, schemaType := range types {
		if agentSchemaTypeMatches(value, schemaType) {
			return true
		}
	}
	return false
}

func agentSchemaTypeMatches(value any, schemaType string) bool {
	switch schemaType {
	case "null":
		return value == nil
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "number":
		return agentJSONNumber(value, false)
	case "integer":
		return agentJSONNumber(value, true)
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	default:
		return false
	}
}

func agentJSONNumber(value any, integer bool) bool {
	var number float64
	switch value := value.(type) {
	case float64:
		number = value
	case float32:
		number = float64(value)
	case int:
		number = float64(value)
	case int8:
		number = float64(value)
	case int16:
		number = float64(value)
	case int32:
		number = float64(value)
	case int64:
		number = float64(value)
	case uint:
		number = float64(value)
	case uint8:
		number = float64(value)
	case uint16:
		number = float64(value)
	case uint32:
		number = float64(value)
	case uint64:
		number = float64(value)
	case json.Number:
		parsed, err := value.Float64()
		if err != nil {
			return false
		}
		number = parsed
	default:
		return false
	}
	return !integer || math.Trunc(number) == number
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
