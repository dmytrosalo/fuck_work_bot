package handlers

import "testing"

func TestPublicBaseURL(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		t.Setenv("PUBLIC_BASE_URL", "")
		if got, want := publicBaseURL(), "https://fuck-work-bot.fly.dev"; got != want {
			t.Errorf("publicBaseURL() = %q, want %q", got, want)
		}
	})

	t.Run("env override", func(t *testing.T) {
		t.Setenv("PUBLIC_BASE_URL", "https://example.com")
		if got, want := publicBaseURL(), "https://example.com"; got != want {
			t.Errorf("publicBaseURL() = %q, want %q", got, want)
		}
	})
}
