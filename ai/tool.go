package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// ConstrainedSampling is the closed set of provider-side sampling settings
// accepted by a Tool. A nil value means the property was absent;
// ConstrainedSamplingDisabled represents an explicit JSON false.
type ConstrainedSampling interface {
	constrainedSampling()
}

// ConstrainedSamplingConfig is the Go mapping of Pi's exported configuration
// union. ConstrainedSampling is the shorter idiomatic name used by Tool.
type ConstrainedSamplingConfig = ConstrainedSampling

// ConstrainedSamplingDisabled represents constrainedSampling: false.
type ConstrainedSamplingDisabled struct{}

func (ConstrainedSamplingDisabled) constrainedSampling() {}

// ConstrainedSamplingStrict selects whether JSON-schema constrained sampling
// is preferred when supported or required for the Tool to be usable.
type ConstrainedSamplingStrict string

const (
	ConstrainedSamplingStrictPrefer  ConstrainedSamplingStrict = "prefer"
	ConstrainedSamplingStrictRequire ConstrainedSamplingStrict = "require"
)

// JSONSchemaConstrainedSampling requests JSON-schema constrained sampling.
type JSONSchemaConstrainedSampling struct {
	Strict ConstrainedSamplingStrict `json:"strict"`
}

func (JSONSchemaConstrainedSampling) constrainedSampling() {}

// GrammarFormat identifies one provider-specific grammar representation.
type GrammarFormat string

const (
	GrammarFormatOpenAILark  GrammarFormat = "openai_lark"
	GrammarFormatOpenAIRegex GrammarFormat = "openai_regex"
)

// GrammarVariants carries the OpenAI Lark and regex representations of the
// same accepted language. Missing entries remain absent.
type GrammarVariants struct {
	OpenAILark  Optional[string] `json:"openai_lark,omitzero"`
	OpenAIRegex Optional[string] `json:"openai_regex,omitzero"`
}

// GrammarConstrainedSampling requests provider grammar constrained sampling.
type GrammarConstrainedSampling struct {
	Variants GrammarVariants `json:"variants"`
}

func (GrammarConstrainedSampling) constrainedSampling() {}

// Tool is model-visible Tool metadata. Parameters is the authoritative raw
// JSON Schema; execution, preparation, and validation belong to the Agent Tool
// dispatcher rather than this metadata contract.
type Tool struct {
	Name                string
	Description         string
	Parameters          json.RawMessage
	ConstrainedSampling ConstrainedSampling
}

// MarshalJSON preserves the closed constrained-sampling union on the wire.
func (t Tool) MarshalJSON() ([]byte, error) {
	if err := validateJSONSchemaRoot(t.Parameters); err != nil {
		return nil, newCodecError("tool parameters", "", err)
	}

	var constrained json.RawMessage
	switch config := t.ConstrainedSampling.(type) {
	case nil:
	case ConstrainedSamplingDisabled:
		constrained = json.RawMessage("false")
	case *ConstrainedSamplingDisabled:
		if config == nil {
			return nil, newCodecError("tool constrained sampling", "false", fmt.Errorf("nil variant"))
		}
		constrained = json.RawMessage("false")
	case JSONSchemaConstrainedSampling:
		if !validConstrainedSamplingStrict(config.Strict) {
			return nil, newCodecError("tool constrained sampling", "json_schema", fmt.Errorf("invalid strict value %q", config.Strict))
		}
		encoded, err := json.Marshal(struct {
			Type   string                    `json:"type"`
			Strict ConstrainedSamplingStrict `json:"strict"`
		}{Type: "json_schema", Strict: config.Strict})
		if err != nil {
			return nil, err
		}
		constrained = encoded
	case *JSONSchemaConstrainedSampling:
		if config == nil {
			return nil, newCodecError("tool constrained sampling", "json_schema", fmt.Errorf("nil variant"))
		}
		return (Tool{Name: t.Name, Description: t.Description, Parameters: t.Parameters, ConstrainedSampling: *config}).MarshalJSON()
	case GrammarConstrainedSampling:
		if err := validateGrammarVariants(config.Variants); err != nil {
			return nil, newCodecError("tool constrained sampling", "grammar", err)
		}
		encoded, err := json.Marshal(struct {
			Type     string          `json:"type"`
			Variants GrammarVariants `json:"variants"`
		}{Type: "grammar", Variants: config.Variants})
		if err != nil {
			return nil, err
		}
		constrained = encoded
	case *GrammarConstrainedSampling:
		if config == nil {
			return nil, newCodecError("tool constrained sampling", "grammar", fmt.Errorf("nil variant"))
		}
		return (Tool{Name: t.Name, Description: t.Description, Parameters: t.Parameters, ConstrainedSampling: *config}).MarshalJSON()
	default:
		return nil, newCodecError("tool constrained sampling", "", fmt.Errorf("unsupported variant %T", config))
	}

	name, err := json.Marshal(t.Name)
	if err != nil {
		return nil, err
	}
	description, err := json.Marshal(t.Description)
	if err != nil {
		return nil, err
	}

	// encoding/json compacts embedded RawMessage values. Assemble the object
	// from individually validated tokens so the authoritative schema bytes are
	// retained exactly rather than normalized or regenerated.
	var encoded bytes.Buffer
	encoded.WriteString(`{"name":`)
	encoded.Write(name)
	encoded.WriteString(`,"description":`)
	encoded.Write(description)
	encoded.WriteString(`,"parameters":`)
	encoded.Write(t.Parameters)
	if constrained != nil {
		encoded.WriteString(`,"constrainedSampling":`)
		encoded.Write(constrained)
	}
	encoded.WriteByte('}')
	return encoded.Bytes(), nil
}

// UnmarshalJSON decodes only the fixed constrained-sampling variants.
func (t *Tool) UnmarshalJSON(data []byte) error {
	fields, err := decodeWireObject(data)
	if err != nil {
		return newCodecError("tool", "", err)
	}
	if err := requireWireString(fields, "name"); err != nil {
		return newCodecError("tool", "", err)
	}
	if err := requireWireString(fields, "description"); err != nil {
		return newCodecError("tool", "", err)
	}
	if _, err := requiredWireField(fields, "parameters"); err != nil {
		return newCodecError("tool parameters", "", err)
	}
	var wire struct {
		Name                string          `json:"name"`
		Description         string          `json:"description"`
		Parameters          json.RawMessage `json:"parameters"`
		ConstrainedSampling json.RawMessage `json:"constrainedSampling"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return newCodecError("tool", "", err)
	}
	if err := validateJSONSchemaRoot(wire.Parameters); err != nil {
		return newCodecError("tool parameters", "", err)
	}

	config, err := unmarshalConstrainedSampling(wire.ConstrainedSampling)
	if err != nil {
		return err
	}
	*t = Tool{
		Name:                wire.Name,
		Description:         wire.Description,
		Parameters:          append(json.RawMessage(nil), wire.Parameters...),
		ConstrainedSampling: config,
	}
	return nil
}

func unmarshalConstrainedSampling(data json.RawMessage) (ConstrainedSampling, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, nil
	}
	if bytes.Equal(trimmed, []byte("false")) {
		return ConstrainedSamplingDisabled{}, nil
	}
	if bytes.Equal(trimmed, []byte("null")) || bytes.Equal(trimmed, []byte("true")) {
		return nil, newCodecError("tool constrained sampling", string(trimmed), nil)
	}

	var discriminator struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(trimmed, &discriminator); err != nil {
		return nil, newCodecError("tool constrained sampling", "", err)
	}
	switch discriminator.Type {
	case "json_schema":
		var wire struct {
			Type   string                    `json:"type"`
			Strict ConstrainedSamplingStrict `json:"strict"`
		}
		if err := json.Unmarshal(trimmed, &wire); err != nil {
			return nil, newCodecError("tool constrained sampling", discriminator.Type, err)
		}
		if !validConstrainedSamplingStrict(wire.Strict) {
			return nil, newCodecError("tool constrained sampling", discriminator.Type, fmt.Errorf("invalid strict value %q", wire.Strict))
		}
		return JSONSchemaConstrainedSampling{Strict: wire.Strict}, nil
	case "grammar":
		var wire struct {
			Type     string          `json:"type"`
			Variants GrammarVariants `json:"variants"`
		}
		if err := json.Unmarshal(trimmed, &wire); err != nil {
			return nil, newCodecError("tool constrained sampling", discriminator.Type, err)
		}
		if err := validateGrammarVariants(wire.Variants); err != nil {
			return nil, newCodecError("tool constrained sampling", discriminator.Type, err)
		}
		return GrammarConstrainedSampling{Variants: wire.Variants}, nil
	default:
		return nil, newCodecError("tool constrained sampling", discriminator.Type, nil)
	}
}

func validConstrainedSamplingStrict(strict ConstrainedSamplingStrict) bool {
	return strict == ConstrainedSamplingStrictPrefer || strict == ConstrainedSamplingStrictRequire
}

func validateGrammarVariants(variants GrammarVariants) error {
	if variants.OpenAILark.IsNull() || variants.OpenAIRegex.IsNull() {
		return fmt.Errorf("grammar variants cannot be null")
	}
	_, hasLark := variants.OpenAILark.Value()
	_, hasRegex := variants.OpenAIRegex.Value()
	if !hasLark && !hasRegex {
		return fmt.Errorf("at least one grammar variant is required")
	}
	return nil
}

// Context is the complete model input plus optional Tool metadata.
type Context struct {
	SystemPrompt Optional[string] `json:"systemPrompt,omitzero"`
	Messages     []Message        `json:"-"`
	Tools        []Tool           `json:"tools,omitempty"`
}
