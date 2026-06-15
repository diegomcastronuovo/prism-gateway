package httpapi

import (
	"testing"
	"time"
)

func TestMonthStartUTC(t *testing.T) {
	d := time.Date(2026, 3, 15, 14, 30, 0, 0, time.UTC)
	got := monthStartUTC(d)
	want := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("monthStartUTC = %v, want %v", got, want)
	}
}

func TestNormalizeModelTypeForMetrics(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"llm", "llm"},
		{"embedding", "embedding"},
		{"ml", "ml"},
		{"", "llm"},
		{"anything_else", "llm"},
	}
	for _, tc := range tests {
		if g := normalizeModelTypeForMetrics(tc.in); g != tc.want {
			t.Errorf("normalizeModelTypeForMetrics(%q) = %q, want %q", tc.in, g, tc.want)
		}
	}
}
