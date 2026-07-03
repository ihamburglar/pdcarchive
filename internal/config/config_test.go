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
