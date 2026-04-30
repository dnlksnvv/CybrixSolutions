package sber

import "testing"

func TestResolveEncoding(t *testing.T) {
	tests := []struct {
		hint, tpl, want string
	}{
		{"pcm_s16le", "pcm16", "pcm16"},
		{"opus", "pcm16", "opus"},
		{"audio/pcm", "wav16", "pcm16"},
		{"wav", "pcm16", "wav"},
	}
	for _, tc := range tests {
		_, media, err := resolveEncoding(tc.hint, tc.tpl)
		if err != nil {
			t.Fatalf("resolveEncoding(%q,%q) err: %v", tc.hint, tc.tpl, err)
		}
		got := ""
		switch media {
		case "audio/pcm":
			got = "pcm16"
		case "audio/wav":
			got = "wav"
		case "audio/opus":
			got = "opus"
		}
		if got != tc.want {
			t.Fatalf("resolveEncoding(%q,%q) media=%q want %q", tc.hint, tc.tpl, got, tc.want)
		}
	}
}
