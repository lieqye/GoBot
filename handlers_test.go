package main

import "testing"

func TestParseAgentAction(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		wantName  string
		wantValue int
		wantOK    bool
	}{
		{name: "ban", query: "ban this user", wantName: "ban", wantOK: true},
		{name: "mute", query: "mute him for 10 minutes", wantName: "mute", wantValue: 10, wantOK: true},
		{name: "purge", query: "purge the last 7 messages", wantName: "purge", wantValue: 7, wantOK: true},
		{name: "no action", query: "tell me a joke", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action, ok := parseAgentAction(tt.query)
			if ok != tt.wantOK {
				t.Fatalf("parseAgentAction(%q) ok=%v want %v", tt.query, ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if action.name != tt.wantName {
				t.Fatalf("parseAgentAction(%q) name=%q want %q", tt.query, action.name, tt.wantName)
			}
			if action.value != tt.wantValue {
				t.Fatalf("parseAgentAction(%q) value=%d want %d", tt.query, action.value, tt.wantValue)
			}
		})
	}
}
