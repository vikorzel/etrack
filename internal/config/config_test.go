package config

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func unsetConfigEnv(t *testing.T) {
	t.Helper()
	keys := []string{
		"PB_CONFIG_HOST",
		"PB_CONFIG_COLLECTION",
		"PB_CONFIG_APP",
		"TELEGRAM_BOT_TOKEN",
		"DB_PATH",
		"BOT_TIMEZONE",
		"WEIGH_IN_INTERVAL",
	}
	for _, k := range keys {
		t.Setenv(k, "")
		if err := os.Unsetenv(k); err != nil {
			t.Fatalf("unset %s: %v", k, err)
		}
	}
}

func TestLoadFromEnvFileAndDefaults(t *testing.T) {
	unsetConfigEnv(t)

	envPath := filepath.Join(t.TempDir(), ".env")
	content := "# comment\n\nTELEGRAM_BOT_TOKEN=\"123:abc\"\n"
	if err := os.WriteFile(envPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	cfg, err := Load(context.Background(), envPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Token != "123:abc" {
		t.Fatalf("Token = %q, want %q", cfg.Token, "123:abc")
	}
	if cfg.DBPath != "meals.db" {
		t.Fatalf("DBPath = %q, want %q", cfg.DBPath, "meals.db")
	}
	if cfg.Location != time.Local {
		t.Fatalf("Location = %v, want time.Local", cfg.Location)
	}
	if cfg.WeighInInterval != DefaultWeighInInterval {
		t.Fatalf("WeighInInterval = %d, want %d", cfg.WeighInInterval, DefaultWeighInInterval)
	}
	if cfg.Source != "env" {
		t.Fatalf("Source = %q, want %q", cfg.Source, "env")
	}
}

func TestLoadEnvOverridesFile(t *testing.T) {
	unsetConfigEnv(t)
	t.Setenv("TELEGRAM_BOT_TOKEN", "env-token")
	t.Setenv("DB_PATH", "/tmp/custom.db")
	t.Setenv("BOT_TIMEZONE", "UTC")
	t.Setenv("WEIGH_IN_INTERVAL", "20")

	envPath := filepath.Join(t.TempDir(), ".env")
	content := "TELEGRAM_BOT_TOKEN='file-token'\nDB_PATH=file.db\nBOT_TIMEZONE=Invalid/Zone\nWEIGH_IN_INTERVAL=bad\n"
	if err := os.WriteFile(envPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	cfg, err := Load(context.Background(), envPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Token != "env-token" {
		t.Fatalf("Token = %q, want %q", cfg.Token, "env-token")
	}
	if cfg.DBPath != "/tmp/custom.db" {
		t.Fatalf("DBPath = %q, want %q", cfg.DBPath, "/tmp/custom.db")
	}
	if cfg.Location.String() != "UTC" {
		t.Fatalf("Location = %q, want UTC", cfg.Location.String())
	}
	if cfg.WeighInInterval != 20 {
		t.Fatalf("WeighInInterval = %d, want 20", cfg.WeighInInterval)
	}
	if cfg.Source != "env" {
		t.Fatalf("Source = %q, want %q", cfg.Source, "env")
	}
}

func TestLoadMissingTokenErrors(t *testing.T) {
	unsetConfigEnv(t)

	_, err := Load(context.Background(), filepath.Join(t.TempDir(), "nonexistent.env"))
	if err == nil {
		t.Fatal("expected error when TELEGRAM_BOT_TOKEN is missing")
	}
}

func TestLoadInvalidTimezoneErrors(t *testing.T) {
	unsetConfigEnv(t)
	t.Setenv("TELEGRAM_BOT_TOKEN", "ok")
	t.Setenv("BOT_TIMEZONE", "Mars/Olympus_Mons")

	_, err := Load(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for invalid BOT_TIMEZONE")
	}
}

func TestLoadWeighInIntervalLocalValidation(t *testing.T) {
	for _, raw := range []string{"abc", "-1", "12.5"} {
		t.Run("invalid_"+raw, func(t *testing.T) {
			unsetConfigEnv(t)
			t.Setenv("TELEGRAM_BOT_TOKEN", "ok")
			t.Setenv("WEIGH_IN_INTERVAL", raw)
			if _, err := Load(context.Background(), ""); err == nil {
				t.Fatalf("expected error for WEIGH_IN_INTERVAL=%q", raw)
			}
		})
	}

	t.Run("zero_uses_default", func(t *testing.T) {
		unsetConfigEnv(t)
		t.Setenv("TELEGRAM_BOT_TOKEN", "ok")
		t.Setenv("WEIGH_IN_INTERVAL", "0")
		cfg, err := Load(context.Background(), "")
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if cfg.WeighInInterval != DefaultWeighInInterval {
			t.Fatalf("WeighInInterval = %d, want %d", cfg.WeighInInterval, DefaultWeighInInterval)
		}
	})
}

func TestLoadFromPocketBaseSuccess(t *testing.T) {
	unsetConfigEnv(t)
	t.Setenv("PB_CONFIG_HOST", "https://pb.internal:8090")
	// Local values must be ignored when PB_CONFIG_HOST is set.
	t.Setenv("TELEGRAM_BOT_TOKEN", "local-ignored-token")
	t.Setenv("DB_PATH", "/ignored/local.db")
	t.Setenv("BOT_TIMEZONE", "Invalid/Ignored")

	var gotReq *http.Request
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			gotReq = req
			return jsonResponse(http.StatusOK, `{
				"items": [{
					"app": "meal-bot",
					"telegram_bot_token": "pb-secret-token",
					"db_path": "/data/meals.db",
					"bot_timezone": "UTC",
					"weigh_in_interval": 10
				}]
			}`), nil
		}),
	}

	cfg, err := loadWithClient(context.Background(), "", client)
	if err != nil {
		t.Fatalf("loadWithClient() error = %v", err)
	}
	if gotReq == nil {
		t.Fatal("expected HTTP request to PocketBase")
	}
	if gotReq.Method != http.MethodGet {
		t.Fatalf("method = %s, want GET", gotReq.Method)
	}
	if gotReq.URL.Scheme != "https" || gotReq.URL.Host != "pb.internal:8090" {
		t.Fatalf("unexpected URL base: %s", gotReq.URL.String())
	}
	if gotReq.URL.Path != "/api/collections/configs/records" {
		t.Fatalf("path = %q, want /api/collections/configs/records", gotReq.URL.Path)
	}
	q := gotReq.URL.Query()
	if q.Get("filter") != "(app='meal-bot')" || q.Get("perPage") != "1" {
		t.Fatalf("unexpected query params: %v", q)
	}
	if cfg.Token != "pb-secret-token" {
		t.Fatalf("Token = %q, want %q", cfg.Token, "pb-secret-token")
	}
	if cfg.DBPath != "/data/meals.db" {
		t.Fatalf("DBPath = %q, want %q", cfg.DBPath, "/data/meals.db")
	}
	if cfg.Location.String() != "UTC" {
		t.Fatalf("Location = %q, want UTC", cfg.Location.String())
	}
	if cfg.WeighInInterval != 10 {
		t.Fatalf("WeighInInterval = %d, want 10", cfg.WeighInInterval)
	}
	if cfg.Source != "pocketbase" {
		t.Fatalf("Source = %q, want %q", cfg.Source, "pocketbase")
	}
}

func TestLoadFromPocketBaseCustomCollectionAppAndDefaults(t *testing.T) {
	unsetConfigEnv(t)
	t.Setenv("PB_CONFIG_HOST", "https://pb.example.com/sub/")
	t.Setenv("PB_CONFIG_COLLECTION", "app_configs")
	t.Setenv("PB_CONFIG_APP", "meal-bot-prod")

	var gotPath, gotFilter string
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			gotPath = req.URL.Path
			gotFilter = req.URL.Query().Get("filter")
			return jsonResponse(http.StatusOK, `{
				"items": [{
					"app": "meal-bot-prod",
					"telegram_bot_token": "pb-prod-token",
					"db_path": "",
					"bot_timezone": ""
				}]
			}`), nil
		}),
	}

	cfg, err := loadWithClient(context.Background(), "", client)
	if err != nil {
		t.Fatalf("loadWithClient() error = %v", err)
	}
	if gotPath != "/sub/api/collections/app_configs/records" {
		t.Fatalf("path = %q, want /sub/api/collections/app_configs/records", gotPath)
	}
	if gotFilter != "(app='meal-bot-prod')" {
		t.Fatalf("filter = %q, want (app='meal-bot-prod')", gotFilter)
	}
	if cfg.DBPath != "meals.db" {
		t.Fatalf("DBPath = %q, want meals.db", cfg.DBPath)
	}
	if cfg.Location != time.Local {
		t.Fatalf("Location = %v, want time.Local", cfg.Location)
	}
	if cfg.WeighInInterval != DefaultWeighInInterval {
		t.Fatalf("WeighInInterval = %d, want %d", cfg.WeighInInterval, DefaultWeighInInterval)
	}
	if cfg.Source != "pocketbase" {
		t.Fatalf("Source = %q, want pocketbase", cfg.Source)
	}
}

func TestLoadFromPocketBaseHostWithoutScheme(t *testing.T) {
	unsetConfigEnv(t)
	t.Setenv("PB_CONFIG_HOST", "pocketbase.internal:8090")

	var gotURL string
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			gotURL = req.URL.String()
			return jsonResponse(http.StatusOK, `{
				"items": [{
					"app": "meal-bot",
					"telegram_bot_token": "tok"
				}]
			}`), nil
		}),
	}

	cfg, err := loadWithClient(context.Background(), "", client)
	if err != nil {
		t.Fatalf("loadWithClient() error = %v", err)
	}
	if !strings.HasPrefix(gotURL, "http://pocketbase.internal:8090/api/collections/configs/records?") {
		t.Fatalf("gotURL = %q", gotURL)
	}
	if cfg.Token != "tok" {
		t.Fatalf("Token = %q, want tok", cfg.Token)
	}
}

func TestLoadFromPocketBaseErrors(t *testing.T) {
	cases := []struct {
		name      string
		host      string
		envToken  string
		transport roundTripFunc
	}{
		{
			name: "invalid scheme",
			host: "ftp://pb.internal:8090",
		},
		{
			name: "missing host",
			host: "http://",
		},
		{
			name: "transport failure",
			host: "http://pb.internal:8090",
			transport: func(*http.Request) (*http.Response, error) {
				return nil, errors.New("dial refused")
			},
		},
		{
			name: "http 500",
			host: "http://pb.internal:8090",
			transport: func(*http.Request) (*http.Response, error) {
				return jsonResponse(http.StatusInternalServerError, `{"message":"boom"}`), nil
			},
		},
		{
			name:     "empty items does not fall back to local env",
			host:     "http://pb.internal:8090",
			envToken: "local-should-not-be-used",
			transport: func(*http.Request) (*http.Response, error) {
				return jsonResponse(http.StatusOK, `{"items":[]}`), nil
			},
		},
		{
			name: "empty token in record",
			host: "http://pb.internal:8090",
			transport: func(*http.Request) (*http.Response, error) {
				return jsonResponse(http.StatusOK, `{"items":[{"app":"meal-bot","telegram_bot_token":"   "}]}`), nil
			},
		},
		{
			name: "invalid timezone in record",
			host: "http://pb.internal:8090",
			transport: func(*http.Request) (*http.Response, error) {
				return jsonResponse(http.StatusOK, `{"items":[{"app":"meal-bot","telegram_bot_token":"tok","bot_timezone":"Mars/Phobos"}]}`), nil
			},
		},
		{
			name: "negative weigh_in_interval in record",
			host: "http://pb.internal:8090",
			transport: func(*http.Request) (*http.Response, error) {
				return jsonResponse(http.StatusOK, `{"items":[{"app":"meal-bot","telegram_bot_token":"tok","weigh_in_interval":-5}]}`), nil
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			unsetConfigEnv(t)
			t.Setenv("PB_CONFIG_HOST", tc.host)
			if tc.envToken != "" {
				t.Setenv("TELEGRAM_BOT_TOKEN", tc.envToken)
			}
			client := &http.Client{Transport: tc.transport}
			if _, err := loadWithClient(context.Background(), "", client); err == nil {
				t.Fatalf("expected error for %s, got nil", tc.name)
			}
		})
	}
}
