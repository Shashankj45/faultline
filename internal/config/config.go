package config

import (
	"flag"
	"os"
	"strconv"
)

type Config struct {
	Port         int
	DatabaseURL  string
	Driver       string
	AdminToken   string
	MockServices bool
	StaticDir    string
}

func Load() *Config {
	cfg := &Config{
		Port:         8080,
		DatabaseURL:  "faultline.db",
		Driver:       "sqlite",
		AdminToken:   "",
		MockServices: true,
		StaticDir:    "web/dist",
	}

	// Environment variables
	if portStr := os.Getenv("PORT"); portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil {
			cfg.Port = p
		}
	}
	if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
		cfg.DatabaseURL = dbURL
		if len(dbURL) >= 8 && dbURL[:8] == "postgres" {
			cfg.Driver = "postgres"
		} else {
			cfg.Driver = "sqlite"
		}
	}
	if token := os.Getenv("FAULTLINE_ADMIN_TOKEN"); token != "" {
		cfg.AdminToken = token
	}
	if mock := os.Getenv("FAULTLINE_MOCK_SERVICES"); mock != "" {
		cfg.MockServices = mock == "true" || mock == "1"
	}

	// Command line flags override
	flag.IntVar(&cfg.Port, "port", cfg.Port, "HTTP server port for Proxy and Control Plane")
	flag.StringVar(&cfg.DatabaseURL, "db", cfg.DatabaseURL, "Database connection string or SQLite file path")
	flag.StringVar(&cfg.Driver, "driver", cfg.Driver, "Database driver: 'sqlite' or 'postgres'")
	flag.StringVar(&cfg.AdminToken, "admin-token", cfg.AdminToken, "Initial admin token")
	flag.BoolVar(&cfg.MockServices, "mock-services", cfg.MockServices, "Start embedded mock services for instant experimentation")
	flag.StringVar(&cfg.StaticDir, "static-dir", cfg.StaticDir, "Directory containing static web UI build")

	// Parse flags only if not already parsed
	if !flag.Parsed() {
		flag.Parse()
	}

	return cfg
}
