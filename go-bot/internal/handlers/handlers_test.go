package handlers

import "testing"

func TestResolveTarget(t *testing.T) {
	tests := []struct {
		name, username, want string
	}{
		{"Danya", "Dany_ro", "Danya"},
		{"Data", "kondzhariia_data", "Data"},
		{"Bo", "facethestrange", "Bo"},
		{"Unknown", "random_user", "Unknown"},
	}
	for _, tt := range tests {
		got := resolveTarget(tt.name, tt.username)
		if got != tt.want {
			t.Errorf("resolveTarget(%q, %q) = %q, want %q", tt.name, tt.username, got, tt.want)
		}
	}
}
