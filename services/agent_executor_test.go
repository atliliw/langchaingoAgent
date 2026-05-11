package services

import "testing"

func TestParseDecision(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"不充分", "不充分"},
		{"充分", "充分"},
		{"信息不充分", "不充分"},
		{"充足", "充分"},
		{"不通过", "不通过"},
		{"通过", "通过"},
		{"tech", "tech"},
		{"general", "general"},
	}
	for _, tt := range tests {
		result := parseDecision(tt.input)
		if result == nil {
			t.Errorf("parseDecision(%q) = nil, want %q", tt.input, tt.expected)
		} else if *result != tt.expected {
			t.Errorf("parseDecision(%q) = %q, want %q", tt.input, *result, tt.expected)
		}
	}
}
