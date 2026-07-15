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
	DatabaseURL      string
	DBMaxOpenConns   int
	DBMaxIdleConns   int
	Port             string
	SourceBaseURL    string
	SocrataAppToken  string
	SyncTimeHour     int
	SyncTimeMinute   int
	SyncTimezone     *time.Location
	SyncPageSize        int
	SyncPageIntervalMin time.Duration
	SyncPageIntervalMax time.Duration
	AdminUsername       string
	AdminPassword       string
	SessionSecret       string
	Production          bool
}

func Load() (*Config, error) {
	envPath := loadDotEnv()

	syncHour, syncMinute, err := parseSyncTime(getEnv("SYNC_TIME", "02:00"))
	if err != nil {
		return nil, fmt.Errorf("SYNC_TIME: %w", err)
	}

	syncTZ, err := time.LoadLocation(getEnv("SYNC_TIMEZONE", "America/Los_Angeles"))
	if err != nil {
		return nil, fmt.Errorf("SYNC_TIMEZONE: %w", err)
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

	syncPageIntervalMin, err := time.ParseDuration(getEnv("SYNC_PAGE_INTERVAL_MIN", "5s"))
	if err != nil {
		syncPageIntervalMin = 5 * time.Second
	}
	syncPageIntervalMax, err := time.ParseDuration(getEnv("SYNC_PAGE_INTERVAL_MAX", "15s"))
	if err != nil {
		syncPageIntervalMax = 15 * time.Second
	}
	if syncPageIntervalMax < syncPageIntervalMin {
		syncPageIntervalMax = syncPageIntervalMin
	}
	dbMaxOpenConns := getPositiveIntEnv("DB_MAX_OPEN_CONNS", 12)
	dbMaxIdleConns := getPositiveIntEnv("DB_MAX_IDLE_CONNS", 5)
	if dbMaxIdleConns > dbMaxOpenConns {
		dbMaxIdleConns = dbMaxOpenConns
	}

	cfg := &Config{
		DatabaseURL:         getEnv("DATABASE_URL", ""),
		DBMaxOpenConns:      dbMaxOpenConns,
		DBMaxIdleConns:      dbMaxIdleConns,
		Port:                getEnv("PORT", "8080"),
		SourceBaseURL:       strings.TrimRight(getEnv("SOURCE_BASE_URL", "https://data.wa.gov"), "/"),
		SocrataAppToken:     getEnv("SOCRATA_APP_TOKEN", ""),
		SyncTimeHour:        syncHour,
		SyncTimeMinute:      syncMinute,
		SyncTimezone:        syncTZ,
		SyncPageSize:        syncPageSize,
		SyncPageIntervalMin: syncPageIntervalMin,
		SyncPageIntervalMax: syncPageIntervalMax,
		AdminUsername:       adminUser,
		AdminPassword:       adminPass,
		SessionSecret:       getEnv("SESSION_SECRET", "dev-secret-change-me"),
		Production:          production,
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

func getPositiveIntEnv(key string, fallback int) int {
	if raw := getEnv(key, ""); raw != "" {
		if n, err := parsePositiveInt(raw); err == nil {
			return n
		}
	}
	return fallback
}

func parseSyncTime(raw string) (hour, minute int, err error) {
	parts := strings.Split(raw, ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid time %q, want HH:MM", raw)
	}

	hour, err = parseHourMinutePart(parts[0], 23, "hour")
	if err != nil {
		return 0, 0, err
	}
	minute, err = parseHourMinutePart(parts[1], 59, "minute")
	if err != nil {
		return 0, 0, err
	}
	return hour, minute, nil
}

func parseHourMinutePart(raw string, max int, label string) (int, error) {
	if raw == "" {
		return 0, fmt.Errorf("invalid %s in time", label)
	}
	n := 0
	for _, c := range raw {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid %s in time", label)
		}
		n = n*10 + int(c-'0')
	}
	if n < 0 || n > max {
		return 0, fmt.Errorf("%s out of range: %d", label, n)
	}
	return n, nil
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
