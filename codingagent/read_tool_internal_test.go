package codingagent

import "testing"

func TestFormatReadToolNumberMatchesJavaScriptBoundaries(t *testing.T) {
	tests := []struct {
		value float64
		want  string
	}{
		{value: 1e20, want: "100000000000000000000"},
		{value: 1e21, want: "1e+21"},
		{value: 1e-6, want: "0.000001"},
		{value: 1e-7, want: "1e-7"},
	}
	for _, test := range tests {
		if got := formatReadToolNumber(test.value); got != test.want {
			t.Errorf("formatReadToolNumber(%v) = %q, want %q", test.value, got, test.want)
		}
	}
}
