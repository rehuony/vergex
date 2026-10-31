package llm

import "testing"

func TestBaseHostLeavesOutCredentials(t *testing.T) {
	for raw, want := range map[string]string{
		"https://user:secret@api.example.com/v1?key=abc": "api.example.com",
		"http://localhost:11434/v1":                      "localhost:11434",
		"not a url":                                      "",
		"":                                               "",
	} {
		if got := BaseHost(raw); got != want {
			t.Errorf("BaseHost(%q) = %q, want %q", raw, got, want)
		}
	}
}
