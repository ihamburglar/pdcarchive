package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseEnvFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := `# comment
ADMIN_USERNAME=admin
ADMIN_PASSWORD="secret123"
EMPTY=
QUOTED='single'
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	vars, err := parseEnvFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if vars["ADMIN_USERNAME"] != "admin" {
		t.Errorf("ADMIN_USERNAME = %q, want admin", vars["ADMIN_USERNAME"])
	}
	if vars["ADMIN_PASSWORD"] != "secret123" {
		t.Errorf("ADMIN_PASSWORD = %q, want secret123", vars["ADMIN_PASSWORD"])
	}
	if vars["QUOTED"] != "single" {
		t.Errorf("QUOTED = %q, want single", vars["QUOTED"])
	}
	if _, ok := vars["EMPTY"]; !ok {
		t.Error("expected EMPTY key to be present")
	}
}

func TestResolveAdminCredsFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("ADMIN_USERNAME=customuser\nADMIN_PASSWORD=mysecret\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("ADMIN_USERNAME", "shelluser")
	t.Setenv("ADMIN_PASSWORD", "shellpass")

	user, pass := resolveAdminCreds(path)
	if user != "customuser" {
		t.Errorf("username = %q, want customuser", user)
	}
	if pass != "mysecret" {
		t.Errorf("password = %q, want mysecret", pass)
	}
}

func TestResolveAdminCredsFallbackToEnv(t *testing.T) {
	t.Setenv("ADMIN_USERNAME", "envuser")
	t.Setenv("ADMIN_PASSWORD", "envpass")

	user, pass := resolveAdminCreds("")
	if user != "envuser" {
		t.Errorf("username = %q, want envuser", user)
	}
	if pass != "envpass" {
		t.Errorf("password = %q, want envpass", pass)
	}
}

func TestValidateAdminPasswordProduction(t *testing.T) {
	tests := []struct {
		name    string
		pass    string
		wantErr bool
	}{
		{"default password", defaultAdminPassword, true},
		{"empty password", "", true},
		{"custom password", "my-secure-pass", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{AdminPassword: tt.pass, Production: true}
			err := validateAdminPassword(cfg)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateAdminPasswordDevAllowsDefault(t *testing.T) {
	cfg := &Config{AdminPassword: defaultAdminPassword, Production: false}
	if err := validateAdminPassword(cfg); err != nil {
		t.Errorf("unexpected error in dev: %v", err)
	}
}

func TestDBPoolDefaults(t *testing.T) {
	t.Setenv("DB_MAX_OPEN_CONNS", "")
	t.Setenv("DB_MAX_IDLE_CONNS", "")

	open := getPositiveIntEnv("DB_MAX_OPEN_CONNS", 12)
	idle := getPositiveIntEnv("DB_MAX_IDLE_CONNS", 5)
	if idle > open {
		idle = open
	}
	if open != 12 {
		t.Fatalf("open conns = %d, want 12", open)
	}
	if idle != 5 {
		t.Fatalf("idle conns = %d, want 5", idle)
	}
}

func TestDBPoolEnvOverride(t *testing.T) {
	t.Setenv("DB_MAX_OPEN_CONNS", "20")
	t.Setenv("DB_MAX_IDLE_CONNS", "7")

	open := getPositiveIntEnv("DB_MAX_OPEN_CONNS", 12)
	idle := getPositiveIntEnv("DB_MAX_IDLE_CONNS", 5)
	if idle > open {
		idle = open
	}
	if open != 20 {
		t.Fatalf("open conns = %d, want 20", open)
	}
	if idle != 7 {
		t.Fatalf("idle conns = %d, want 7", idle)
	}
}

func TestDBPoolIdleClampedToOpen(t *testing.T) {
	t.Setenv("DB_MAX_OPEN_CONNS", "4")
	t.Setenv("DB_MAX_IDLE_CONNS", "9")

	open := getPositiveIntEnv("DB_MAX_OPEN_CONNS", 12)
	idle := getPositiveIntEnv("DB_MAX_IDLE_CONNS", 5)
	if idle > open {
		idle = open
	}
	if idle != 4 {
		t.Fatalf("idle conns = %d, want 4", idle)
	}
}

func TestParseSyncTime(t *testing.T) {
	tests := []struct {
		raw       string
		wantHour  int
		wantMin   int
		wantError bool
	}{
		{"02:00", 2, 0, false},
		{"2:00", 2, 0, false},
		{"14:30", 14, 30, false},
		{"24:00", 0, 0, true},
		{"02:60", 0, 0, true},
		{"bad", 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			hour, minute, err := parseSyncTime(tt.raw)
			if tt.wantError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if hour != tt.wantHour || minute != tt.wantMin {
				t.Fatalf("got %02d:%02d, want %02d:%02d", hour, minute, tt.wantHour, tt.wantMin)
			}
		})
	}
}

func TestUnquoteEnvValue(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{`"hello"`, "hello"},
		{`'world'`, "world"},
		{"plain", "plain"},
	}
	for _, tt := range tests {
		if got := unquoteEnvValue(tt.in); got != tt.want {
			t.Errorf("unquoteEnvValue(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
