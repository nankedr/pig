package parity_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"testing"
	"unicode/utf16"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/internal/parity"
)

func TestMessageHandoffParity(t *testing.T) {
	fixture, locked := loadDeferredToolsFixture(t, "message-handoff.json")
	oracle, err := parity.NewFixtureDriver(fixture, locked)
	if err != nil {
		t.Fatal(err)
	}
	pig := parity.DriverFunc{SurfaceName: parity.SurfaceGoSDK, ObserveFunc: observeMessageHandoff}
	result, err := parity.RunCase(context.Background(), fixture.Case, oracle, pig)
	if err != nil || !result.Match || len(result.Normalizations) != 0 {
		var want, got []map[string]any
		_ = json.Unmarshal(result.Oracle.Outcome, &want)
		_ = json.Unmarshal(result.Pig.Outcome, &got)
		for i := range min(len(want), len(got)) {
			if !reflect.DeepEqual(want[i], got[i]) {
				t.Errorf("scenario %s: got %#v, want %#v", want[i]["id"], got[i], want[i])
			}
		}
		t.Fatalf("message handoff parity: %v, %v", result.Differences, err)
	}
}

func observeMessageHandoff(_ context.Context, declaration parity.Case) (parity.Observation, error) {
	var input struct {
		Scenarios []struct {
			ID        string            `json:"id"`
			Model     ai.Model          `json:"model"`
			Normalize string            `json:"normalize"`
			Wire      bool              `json:"wire"`
			Messages  []json.RawMessage `json:"messages"`
		} `json:"scenarios"`
	}
	if err := json.Unmarshal(declaration.Input, &input); err != nil {
		return parity.Observation{}, err
	}
	outcomes := []any{}
	for _, scenario := range input.Scenarios {
		messages := make([]ai.Message, len(scenario.Messages))
		for i, raw := range scenario.Messages {
			message, err := handoffInputMessage(raw)
			if err != nil {
				return parity.Observation{}, fmt.Errorf("%s: %w", scenario.ID, err)
			}
			messages[i] = message
		}
		before, err := json.Marshal(messages)
		if err != nil {
			return parity.Observation{}, err
		}
		calls := []any{}
		var normalize ai.ToolCallIDNormalizer
		if scenario.Normalize != "none" {
			normalize = func(id string, model ai.Model, source ai.AssistantMessage) string {
				calls = append(calls, map[string]any{"id": id, "target": model.ID, "source": source.Model, "api": source.API, "provider": source.Provider})
				if scenario.Normalize == "identity" {
					return id
				}
				var normalized []rune
				allowed := regexp.MustCompile(`^[a-zA-Z0-9_-]$`)
				for _, unit := range utf16.Encode([]rune(id)) {
					character := rune(unit)
					if !allowed.MatchString(string(character)) {
						character = '_'
					}
					normalized = append(normalized, character)
				}
				return string(normalized[:min(64, len(normalized))])
			}
		}
		var output any
		if scenario.Wire {
			output, err = ai.ConvertOpenAICompletionsMessages(scenario.Model, ai.Context{Messages: messages}, ai.OpenAICompletionsCompat{})
		} else {
			var transformed []ai.Message
			transformed, err = ai.TransformMessages(messages, scenario.Model, normalize)
			for i, message := range transformed {
				if result, ok := message.(ai.ToolResultMessage); ok && result.Timestamp > 5 {
					result.Timestamp = 0
					transformed[i] = result
				}
			}
			output = transformed
		}
		if err != nil {
			return parity.Observation{}, fmt.Errorf("%s: %w", scenario.ID, err)
		}
		after, err := json.Marshal(messages)
		if err != nil || !bytes.Equal(before, after) {
			return parity.Observation{}, fmt.Errorf("%s mutated its source", scenario.ID)
		}
		outcomes = append(outcomes, map[string]any{"id": scenario.ID, "messages": output, "calls": calls})
	}
	outcome, err := json.Marshal(outcomes)
	sideEffects := []parity.SideEffect{}
	return parity.Observation{Outcome: outcome, SideEffects: &sideEffects}, err
}

func handoffInputMessage(raw json.RawMessage) (ai.Message, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	if content := fields["content"]; len(content) > 0 && string(content) != "null" {
		return ai.UnmarshalMessage(raw)
	}
	// Untyped Pi null/missing content is represented by Go zero-value content.
	fields["content"] = json.RawMessage(`[]`)
	encoded, err := json.Marshal(fields)
	if err != nil {
		return nil, err
	}
	message, err := ai.UnmarshalMessage(encoded)
	if err != nil {
		return nil, err
	}
	switch value := message.(type) {
	case ai.UserMessage:
		value.Content = ai.UserMessageContent{}
		return value, nil
	case ai.AssistantMessage:
		value.Content = nil
		return value, nil
	case ai.ToolResultMessage:
		value.Content = nil
		return value, nil
	}
	return message, nil
}
