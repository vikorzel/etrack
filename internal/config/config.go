package config

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultWeighInInterval is the default number of meal records between weigh-in prompts.
	DefaultWeighInInterval = 15

	defaultDBPath       = "meals.db"
	defaultPBCollection = "configs"
	defaultPBApp        = "meal-bot"
)

// Config holds runtime settings for the Telegram meal-tracking bot.
type Config struct {
	Token           string
	DBPath          string
	Location        *time.Location
	WeighInInterval int
	Source          string
}

type pbRecordsResponse struct {
	Items []pbConfigRecord `json:"items"`
}

type pbConfigRecord struct {
	App              string `json:"app"`
	TelegramBotToken string `json:"telegram_bot_token"`
	DBPath           string `json:"db_path"`
	BotTimezone      string `json:"bot_timezone"`
	WeighInInterval  int    `json:"weigh_in_interval"`
}

// Load reads configuration from PocketBase when PB_CONFIG_HOST is set,
// or falls back to local environment variables / envPath otherwise.
func Load(ctx context.Context, envPath string) (Config, error) {
	return loadWithClient(ctx, envPath, &http.Client{Timeout: 10 * time.Second})
}

func loadWithClient(ctx context.Context, envPath string, client *http.Client) (Config, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	fileEnv := map[string]string{}
	if envPath != "" {
		parsed, err := parseEnvFile(envPath)
		if err != nil {
			return Config{}, err
		}
		fileEnv = parsed
	}

	lookup := func(key string) string {
		if val, ok := os.LookupEnv(key); ok {
			return strings.TrimSpace(val)
		}
		return strings.TrimSpace(fileEnv[key])
	}

	if pbHost := lookup("PB_CONFIG_HOST"); pbHost != "" {
		collection := lookup("PB_CONFIG_COLLECTION")
		if collection == "" {
			collection = defaultPBCollection
		}
		appName := lookup("PB_CONFIG_APP")
		if appName == "" {
			appName = defaultPBApp
		}
		return loadFromPocketBase(ctx, client, pbHost, collection, appName)
	}

	weighInInterval := 0
	if rawInterval := lookup("WEIGH_IN_INTERVAL"); rawInterval != "" {
		parsed, err := strconv.Atoi(rawInterval)
		if err != nil {
			return Config{}, fmt.Errorf("invalid WEIGH_IN_INTERVAL %q: must be an integer", rawInterval)
		}
		weighInInterval = parsed
	}

	return buildConfig(
		lookup("TELEGRAM_BOT_TOKEN"),
		lookup("DB_PATH"),
		lookup("BOT_TIMEZONE"),
		weighInInterval,
		"env",
	)
}

func loadFromPocketBase(ctx context.Context, client *http.Client, host, collection, appName string) (Config, error) {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	endpoint, err := normalizePBEndpoint(host, collection, appName)
	if err != nil {
		return Config{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Config{}, fmt.Errorf("create pocketbase request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return Config{}, fmt.Errorf("fetch config from pocketbase: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		msg := strings.TrimSpace(string(errBody))
		if msg == "" {
			return Config{}, fmt.Errorf("pocketbase returned HTTP %d", resp.StatusCode)
		}
		return Config{}, fmt.Errorf("pocketbase returned HTTP %d: %s", resp.StatusCode, msg)
	}

	var payload pbRecordsResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return Config{}, fmt.Errorf("decode pocketbase response: %w", err)
	}
	if len(payload.Items) == 0 {
		return Config{}, fmt.Errorf("pocketbase config record not found for app=%q in collection %q", appName, collection)
	}

	rec := payload.Items[0]
	return buildConfig(rec.TelegramBotToken, rec.DBPath, rec.BotTimezone, rec.WeighInInterval, "pocketbase")
}

func normalizePBEndpoint(host, collection, appName string) (string, error) {
	raw := strings.TrimSpace(host)
	if raw == "" {
		return "", errors.New("PB_CONFIG_HOST must not be empty")
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}

	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid PB_CONFIG_HOST %q: %w", host, err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("invalid PB_CONFIG_HOST %q: expected http(s) URL with host", host)
	}
	if strings.Contains(collection, "/") {
		return "", fmt.Errorf("invalid PB_CONFIG_COLLECTION %q", collection)
	}

	basePath := strings.TrimRight(u.Path, "/")
	u.Path = basePath + "/api/collections/" + collection + "/records"
	u.RawPath = ""

	escapedApp := strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(appName)
	q := u.Query()
	q.Set("filter", fmt.Sprintf("(app='%s')", escapedApp))
	q.Set("perPage", "1")
	u.RawQuery = q.Encode()

	return u.String(), nil
}

func buildConfig(token, dbPath, tz string, weighInInterval int, source string) (Config, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		if source == "pocketbase" {
			return Config{}, errors.New("telegram_bot_token is empty in PocketBase config record")
		}
		return Config{}, errors.New("TELEGRAM_BOT_TOKEN is required (set it in environment or .env)")
	}

	dbPath = strings.TrimSpace(dbPath)
	if dbPath == "" {
		dbPath = defaultDBPath
	}

	loc := time.Local
	if tz = strings.TrimSpace(tz); tz != "" {
		loaded, err := time.LoadLocation(tz)
		if err != nil {
			return Config{}, fmt.Errorf("invalid BOT_TIMEZONE %q: %w", tz, err)
		}
		loc = loaded
	}

	if weighInInterval < 0 {
		if source == "pocketbase" {
			return Config{}, fmt.Errorf("invalid weigh_in_interval %d in PocketBase config record: must be >= 0", weighInInterval)
		}
		return Config{}, fmt.Errorf("invalid WEIGH_IN_INTERVAL %d: must be >= 0", weighInInterval)
	}
	if weighInInterval == 0 {
		weighInInterval = DefaultWeighInInterval
	}

	return Config{
		Token:           token,
		DBPath:          dbPath,
		Location:        loc,
		WeighInInterval: weighInInterval,
		Source:          source,
	}, nil
}

func parseEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("open env file %q: %w", path, err)
	}
	defer f.Close()

	values := make(map[string]string)
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, val, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("invalid env line %d in %q: missing '='", lineNum, path)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("invalid env line %d in %q: empty key", lineNum, path)
		}

		val = stripMatchingQuotes(strings.TrimSpace(val))
		values[key] = val
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read env file %q: %w", path, err)
	}
	return values, nil
}

func stripMatchingQuotes(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
