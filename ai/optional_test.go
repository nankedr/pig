package ai_test

import (
	"encoding/json"
	"testing"

	"github.com/nankedr/pig/ai"
)

func TestOptionalJSONPreservesAbsentNullAndZero(t *testing.T) {
	t.Parallel()

	type document struct {
		Value ai.Optional[int] `json:"value,omitzero"`
	}

	tests := []struct {
		name     string
		input    document
		wantJSON string
		wantSet  bool
		wantNull bool
		want     int
		wantOK   bool
	}{
		{name: "absent", input: document{Value: ai.Absent[int]()}, wantJSON: `{}`},
		{name: "null", input: document{Value: ai.Null[int]()}, wantJSON: `{"value":null}`, wantSet: true, wantNull: true},
		{name: "explicit zero", input: document{Value: ai.Some(0)}, wantJSON: `{"value":0}`, wantSet: true, want: 0, wantOK: true},
		{name: "value", input: document{Value: ai.Some(42)}, wantJSON: `{"value":42}`, wantSet: true, want: 42, wantOK: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			encoded, err := json.Marshal(test.input)
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}
			if got := string(encoded); got != test.wantJSON {
				t.Fatalf("Marshal() = %s, want %s", got, test.wantJSON)
			}

			var decoded document
			if err := json.Unmarshal(encoded, &decoded); err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}
			if got := decoded.Value.IsSet(); got != test.wantSet {
				t.Fatalf("IsSet() = %t, want %t", got, test.wantSet)
			}
			if got := decoded.Value.IsNull(); got != test.wantNull {
				t.Fatalf("IsNull() = %t, want %t", got, test.wantNull)
			}
			got, ok := decoded.Value.Value()
			if ok != test.wantOK || got != test.want {
				t.Fatalf("Value() = (%d, %t), want (%d, %t)", got, ok, test.want, test.wantOK)
			}
		})
	}
}
