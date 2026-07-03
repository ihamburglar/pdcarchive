package config

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

const defaultAdminPassword = "change-me"

type Config struct {
	DatabaseURL     string
	Port            string
	SourceBaseURL   string
	SocrataAppToken string
	Datasets        []string
	SyncInterval    time.Duration
	SyncPageSize    int
	SyncPageInterval time.Duration
	AdminUsername   string
	AdminPassword   string
	SessionSecret   string
	Production      bool
}

func Load() (*Config, error) {
	envPath := loadDotEnv()

	syncInterval, err := time.ParseDuration(getEnv("SYNC_INTERVAL", "24h"))
	if err != nil {
		syncInterval = 24 * time.Hour
	}

	datasets := strings.Split(getEnv("DATASETS", ""), ",")
	var cleaned []string
	for _, d := range datasets {
		d = strings.TrimSpace(d)
		if d != "" {
			cleaned = append(cleaned, d)
		}
	}

	ginMode := getEnv("GIN_MODE", "debug")
	production := ginMode == "release" || os.Getenv("RAILWAY_ENVIRONMENT") != ""

	adminUser, adminPass := resolveAdminCreds(envPath)

	syncPageSize := 1000
	if raw := getEnv("SYNC_PAGE_SIZE", "1000"); raw != "" {
		if n, err := parsePositiveInt(raw); err == nil {
			syncPageSize = n
		}
	}

	syncPageInterval, err := time.ParseDuration(getEnv("SYNC_PAGE_INTERVAL", "1s"))
	if err != nil {
		syncPageInterval = time.Second
	}

	cfg := &Config{
		DatabaseURL:      getEnv("DATABASE_URL", ""),
		Port:             getEnv("PORT", "8080"),
		SourceBaseURL:    strings.TrimRight(getEnv("SOURCE_BASE_URL", "https://data.wa.gov"), "/"),
		SocrataAppToken:  getEnv("SOCRATA_APP_TOKEN", ""),
		Datasets:         cleaned,
		SyncInterval:     syncInterval,
		SyncPageSize:     syncPageSize,
		SyncPageInterval: syncPageInterval,
		AdminUsername:    adminUser,
		AdminPassword:    adminPass,
		SessionSecret:    getEnv("SESSION_SECRET", "dev-secret-change-me"),
		Production:       production,
	}

	if err := validateAdminPassword(cfg); err != nil {
		return nil, err
	}

	logAdminConfig(envPath, adminUser, adminPass)

	return cfg, nil
}

func resolveAdminCreds(envPath string) (username, password string) {
	username = getEnv("ADMIN_USERNAME", "admin")
	password = getEnv("ADMIN_PASSWORD", defaultAdminPassword)

	if envPath == "" {
		return username, password
	}

	fileVars, err := parseEnvFile(envPath)
	if err != nil {
		log.Printf("config: could not parse %q for admin creds: %v", envPath, err)
		return username, password
	}

	if v, ok := fileVars["ADMIN_USERNAME"]; ok && v != "" {
		username = v
	}
	if v, ok := fileVars["ADMIN_PASSWORD"]; ok && v != "" {
		password = v
	}

	return username, password
}

func validateAdminPassword(cfg *Config) error {
	if !cfg.Production {
		return nil
	}
	if cfg.AdminPassword == "" || cfg.AdminPassword == defaultAdminPassword {
		return fmt.Errorf("ADMIN_PASSWORD must be set to a non-default value in production")
	}
	return nil
}

func logAdminConfig(envPath, adminUser, adminPass string) {
	if envPath == "" {
		log.Printf("config: no .env file found")
	} else {
		log.Printf("config: loaded %s", envPath)
	}

	if adminPass == defaultAdminPassword {
		if envPath != "" {
			log.Printf("config: warning — ADMIN_PASSWORD is still the default in %s", envPath)
		} else {
			log.Printf("config: warning — no .env file found; using default ADMIN_PASSWORD")
		}
		log.Printf(`config: admin user %q, password=DEFAULT — set ADMIN_PASSWORD in .env`, adminUser)
		return
	}

	log.Printf(`config: admin user %q, password=custom (%d chars)`, adminUser, len(adminPass))
}

// loadDotEnv finds and loads .env from the working directory, parent directories,
// or next to the executable. When a file is found, Overload is used so .env values
// take precedence over empty shell exports during local development.
// Returns the path of the loaded file, or empty string if none found.
func loadDotEnv() string {
	if explicit := os.Getenv("ENV_FILE"); explicit != "" {
		if err := godotenv.Overload(explicit); err != nil {
			log.Printf("config: could not load ENV_FILE %q: %v", explicit, err)
			return ""
		}
		return explicit
	}

	for _, path := range dotEnvCandidates() {
		if _, err := os.Stat(path); err != nil {
			continue
		}
		if err := godotenv.Overload(path); err != nil {
			log.Printf("config: could not load %q: %v", path, err)
			continue
		}
		return path
	}
	return ""
}

func dotEnvCandidates() []string {
	seen := make(map[string]bool)
	var paths []string

	add := func(path string) {
		if path == "" || seen[path] {
			return
		}
		seen[path] = true
		paths = append(paths, path)
	}

	add(".env")

	if wd, err := os.Getwd(); err == nil {
		dir := wd
		for i := 0; i < 6; i++ {
			add(filepath.Join(dir, ".env"))
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		for i := 0; i < 4; i++ {
			add(filepath.Join(dir, ".env"))
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	return paths
}

// parseEnvFile reads KEY=VALUE pairs from a .env file.
func parseEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	vars := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = unquoteEnvValue(value)
		vars[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return vars, nil
}

func unquoteEnvValue(value string) string {
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') ||
			(value[0] == '\'' && value[len(value)-1] == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}

func getEnv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func parsePositiveInt(s string) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid integer: %s", s)
		}
		n = n*10 + int(c-'0')
	}
	if n <= 0 {
		return 0, fmt.Errorf("must be positive: %s", s)
	}
	return n, nil
}
