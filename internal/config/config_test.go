package config

import (
	"testing"
)

// setRequired fills in everything Load insists on, so each test only has to
// vary the one variable it cares about.
func setRequired(t *testing.T) {
	t.Helper()
	t.Setenv("BOT_TOKEN", "123456:test-token")
}

func TestLoadParsesAllowedUsers(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  []int64
	}{
		{
			// The exact value carried in k8s/secrets.yaml.
			name:  "comma separated ids",
			value: "888397843,685751256",
			want:  []int64{888397843, 685751256},
		},
		{
			name:  "single id",
			value: "888397843",
			want:  []int64{888397843},
		},
		{
			name:  "unset means the bot is public",
			value: "",
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setRequired(t)
			t.Setenv("ALLOWED_USERS", tt.value)

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}

			if len(cfg.AllowedUsers) != len(tt.want) {
				t.Fatalf("AllowedUsers = %v, want %v", cfg.AllowedUsers, tt.want)
			}
			for i, id := range tt.want {
				if cfg.AllowedUsers[i] != id {
					t.Errorf("AllowedUsers[%d] = %d, want %d", i, cfg.AllowedUsers[i], id)
				}
			}
		})
	}
}

// A stray space is worth failing loudly over: a silently dropped ID would lock
// someone out of the bot with no indication why.
func TestLoadRejectsMalformedAllowedUsers(t *testing.T) {
	setRequired(t)
	t.Setenv("ALLOWED_USERS", "888397843, 685751256")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want a parse failure for the spaced value")
	}
	t.Logf("rejected as expected: %v", err)
}
