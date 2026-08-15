// Package config reads the environment into a typed struct and validates it at
// boot. Every process refuses to start on a missing or malformed value, rather
// than discovering it at 3am on the first webhook.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Env string

const (
	EnvDevelopment Env = "development"
	EnvStaging     Env = "staging"
	EnvProduction  Env = "production"
)

func (e Env) IsProduction() bool { return e == EnvProduction }

type Config struct {
	DatabaseURL string
	Port        int
	Env         Env

	SessionCookieDomain string

	FootballDataToken string
	APIFootballKey    string

	MarzPayAPIUser        string
	MarzPayAPIKey         string
	MarzPayWebhookSecret  string
	MarzPayBaseURL        string
	PublicBaseURL         string
	ModelVersion          string
	AllowedOrigins        []string
	LogLevel              string
}

const defaultMarzPayBaseURL = "https://wallet.wearemarz.com/api/v1"

// Load reads and validates the environment. It collects every problem before
// returning, so a misconfigured deploy reports all of its faults in one pass
// instead of one per restart.
func Load() (*Config, error) {
	var problems []string

	cfg := &Config{
		DatabaseURL:          os.Getenv("DATABASE_URL"),
		Env:                  Env(getDefault("ENV", string(EnvDevelopment))),
		SessionCookieDomain:  os.Getenv("SESSION_COOKIE_DOMAIN"),
		FootballDataToken:    os.Getenv("FOOTBALL_DATA_TOKEN"),
		APIFootballKey:       os.Getenv("APIFOOTBALL_KEY"),
		MarzPayAPIUser:       os.Getenv("MARZPAY_API_USER"),
		MarzPayAPIKey:        os.Getenv("MARZPAY_API_KEY"),
		MarzPayWebhookSecret: os.Getenv("MARZPAY_WEBHOOK_SECRET"),
		MarzPayBaseURL:       getDefault("MARZPAY_BASE_URL", defaultMarzPayBaseURL),
		PublicBaseURL:        os.Getenv("PUBLIC_BASE_URL"),
		ModelVersion:         os.Getenv("MODEL_VERSION"),
		LogLevel:             getDefault("LOG_LEVEL", "info"),
	}

	if cfg.DatabaseURL == "" {
		problems = append(problems, "DATABASE_URL is required")
	}

	port, err := strconv.Atoi(getDefault("PORT", "8080"))
	if err != nil || port <= 0 || port > 65535 {
		problems = append(problems, fmt.Sprintf("PORT %q is not a valid port", os.Getenv("PORT")))
	}
	cfg.Port = port

	switch cfg.Env {
	case EnvDevelopment, EnvStaging, EnvProduction:
	default:
		problems = append(problems, fmt.Sprintf(
			"ENV %q must be development, staging or production", cfg.Env))
	}

	if cfg.ModelVersion == "" {
		// Stamped onto every prediction, and the accuracy dashboard reports
		// per version. An unstamped prediction cannot be attributed to the
		// model that made it.
		problems = append(problems, "MODEL_VERSION is required")
	}

	if u := cfg.PublicBaseURL; u == "" {
		problems = append(problems, "PUBLIC_BASE_URL is required (used to build callback_url)")
	} else if parsed, err := url.Parse(u); err != nil || parsed.Scheme == "" || parsed.Host == "" {
		problems = append(problems, fmt.Sprintf("PUBLIC_BASE_URL %q is not an absolute URL", u))
	} else if strings.HasSuffix(u, "/") {
		problems = append(problems, "PUBLIC_BASE_URL must not have a trailing slash")
	}

	if _, err := url.Parse(cfg.MarzPayBaseURL); err != nil {
		problems = append(problems, fmt.Sprintf("MARZPAY_BASE_URL %q is not a URL", cfg.MarzPayBaseURL))
	}

	cfg.AllowedOrigins = splitList(os.Getenv("ALLOWED_ORIGINS"))
	if len(cfg.AllowedOrigins) == 0 {
		// CORS is the Next.js origin only. There is no wildcard fallback:
		// defaulting to "*" on a missing value is how a permissive CORS policy
		// reaches production unnoticed.
		problems = append(problems, "ALLOWED_ORIGINS is required (comma-separated Next.js origins)")
	}

	// Secrets are only mandatory in production. Development runs against a
	// fake payment provider and recorded ingestion fixtures, and demanding
	// live credentials to boot locally is what pushes people to paste real
	// ones into a .env they then commit.
	if cfg.Env.IsProduction() {
		for _, required := range []struct{ name, value string }{
			{"FOOTBALL_DATA_TOKEN", cfg.FootballDataToken},
			{"APIFOOTBALL_KEY", cfg.APIFootballKey},
			{"MARZPAY_API_USER", cfg.MarzPayAPIUser},
			{"MARZPAY_API_KEY", cfg.MarzPayAPIKey},
			{"MARZPAY_WEBHOOK_SECRET", cfg.MarzPayWebhookSecret},
			{"SESSION_COOKIE_DOMAIN", cfg.SessionCookieDomain},
		} {
			if required.value == "" {
				problems = append(problems, required.name+" is required in production")
			}
		}
	}

	if len(problems) > 0 {
		return nil, errors.New("invalid configuration:\n  - " + strings.Join(problems, "\n  - "))
	}
	return cfg, nil
}

// MarzPayConfigured reports whether the live payment client can be built. When
// false the worker and API wire the fake provider instead, which is the only
// way a test can never reach the live API.
func (c *Config) MarzPayConfigured() bool {
	return c.MarzPayAPIUser != "" && c.MarzPayAPIKey != ""
}

func (c *Config) WebhookCallbackURL() string {
	return c.PublicBaseURL + "/v1/webhooks/marzpay"
}

func getDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}
