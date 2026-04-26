package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const defaultConfigPath = "configs/local.yaml"

// Config holds application configuration loaded from YAML with optional env overrides.
type Config struct {
	App           AppConfig           `yaml:"app"`
	Postgres      PostgresConfig      `yaml:"postgres"`
	Redis         RedisConfig         `yaml:"redis"`
	JWT           JWTConfig           `yaml:"jwt"`
	RateLimit     RateLimitConfig     `yaml:"rate_limit"`
	Kafka         KafkaConfig         `yaml:"kafka"`
	WS            WSConfig            `yaml:"ws"`
	Runtime       RuntimeConfig     `yaml:"runtime"`
	Observability ObservabilityConfig `yaml:"observability"`
}

// ObservabilityConfig controls structured logging fields (not Prometheus scrape targets).
type ObservabilityConfig struct {
	ServiceName string `yaml:"service_name"`
	Env         string `yaml:"env"`
	LogFormat   string `yaml:"log_format"` // json | text
}

// RuntimeConfig controls process bootstrap behaviour.
type RuntimeConfig struct {
	RunMigrationsOnStartup bool `yaml:"run_migrations_on_startup"`
}

// KafkaConfig configures optional Kafka domain-event pipeline.
type KafkaConfig struct {
	Enabled       bool   `yaml:"enabled"`
	Brokers       string `yaml:"brokers"` // comma-separated
	Topic         string `yaml:"topic"`
	ConsumerGroup string `yaml:"consumer_group"`
}

// WSConfig controls browser WebSocket safety.
type WSConfig struct {
	AllowedOrigins   []string `yaml:"allowed_origins"`
	TicketTTLSeconds int      `yaml:"ticket_ttl_seconds"`
}

// RateLimitConfig holds per-route sustained limits (requests per minute, per client IP).
type RateLimitConfig struct {
	LoginPerMinute       int `yaml:"login_per_minute"`
	RegisterPerMinute    int `yaml:"register_per_minute"`
	MessageSendPerMinute int `yaml:"message_send_per_minute"`
	WSConnectPerMinute   int `yaml:"ws_connect_per_minute"`
}

type AppConfig struct {
	Port string `yaml:"port"`
}

type PostgresConfig struct {
	DSN string `yaml:"dsn"`
}

type RedisConfig struct {
	Addr string `yaml:"addr"`
}

type JWTConfig struct {
	Secret            string `yaml:"secret"`
	AccessTTLSeconds  int    `yaml:"access_ttl_seconds"`
	RefreshTTLSeconds int    `yaml:"refresh_ttl_seconds"`
}

// Load reads YAML from CONFIG_PATH (default configs/local.yaml), then applies environment overrides.
func Load() (*Config, error) {
	path := strings.TrimSpace(os.Getenv("CONFIG_PATH"))
	if path == "" {
		path = defaultConfigPath
	}

	cfg := &Config{}
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("read config file %q: %w", path, err)
		}
		cfg = &Config{}
	} else {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parse yaml: %w", err)
		}
	}

	applyEnvOverrides(cfg)

	cfg.applyRateLimitDefaults()
	cfg.applyKafkaDefaults()
	cfg.applyObservabilityDefaults()

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) applyRateLimitDefaults() {
	if c == nil {
		return
	}
	if c.RateLimit.LoginPerMinute <= 0 {
		c.RateLimit.LoginPerMinute = 60
	}
	if c.RateLimit.RegisterPerMinute <= 0 {
		c.RateLimit.RegisterPerMinute = 20
	}
	if c.RateLimit.MessageSendPerMinute <= 0 {
		c.RateLimit.MessageSendPerMinute = 120
	}
	if c.RateLimit.WSConnectPerMinute <= 0 {
		c.RateLimit.WSConnectPerMinute = 30
	}
}

func (c *Config) applyKafkaDefaults() {
	if c == nil {
		return
	}
	if strings.TrimSpace(c.Kafka.Topic) == "" {
		c.Kafka.Topic = "goflow.domain.events"
	}
	if strings.TrimSpace(c.Kafka.ConsumerGroup) == "" {
		c.Kafka.ConsumerGroup = "goflow-ws-fanout"
	}
	if c.WS.TicketTTLSeconds <= 0 {
		c.WS.TicketTTLSeconds = 120
	}
}

func applyEnvOverrides(c *Config) {
	if s := strings.TrimSpace(os.Getenv("HTTP_PORT")); s != "" {
		c.App.Port = s
	}
	if s := strings.TrimSpace(os.Getenv("POSTGRES_DSN")); s != "" {
		c.Postgres.DSN = s
	}
	if s := strings.TrimSpace(os.Getenv("REDIS_ADDR")); s != "" {
		c.Redis.Addr = s
	}
	if s := strings.TrimSpace(os.Getenv("JWT_SECRET")); s != "" {
		c.JWT.Secret = s
	}
	if s := strings.TrimSpace(os.Getenv("JWT_ACCESS_TTL_SECONDS")); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			c.JWT.AccessTTLSeconds = n
		}
	}
	if s := strings.TrimSpace(os.Getenv("JWT_REFRESH_TTL_SECONDS")); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			c.JWT.RefreshTTLSeconds = n
		}
	}
	if s := strings.TrimSpace(os.Getenv("RATE_LIMIT_LOGIN_PER_MINUTE")); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			c.RateLimit.LoginPerMinute = n
		}
	}
	if s := strings.TrimSpace(os.Getenv("RATE_LIMIT_REGISTER_PER_MINUTE")); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			c.RateLimit.RegisterPerMinute = n
		}
	}
	if s := strings.TrimSpace(os.Getenv("RATE_LIMIT_MESSAGE_SEND_PER_MINUTE")); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			c.RateLimit.MessageSendPerMinute = n
		}
	}
	if s := strings.TrimSpace(os.Getenv("RATE_LIMIT_WS_CONNECT_PER_MINUTE")); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			c.RateLimit.WSConnectPerMinute = n
		}
	}
	if s := strings.TrimSpace(os.Getenv("KAFKA_ENABLED")); s != "" {
		c.Kafka.Enabled = strings.EqualFold(s, "true") || s == "1"
	}
	if s := strings.TrimSpace(os.Getenv("KAFKA_BROKERS")); s != "" {
		c.Kafka.Brokers = s
	}
	if s := strings.TrimSpace(os.Getenv("KAFKA_TOPIC")); s != "" {
		c.Kafka.Topic = s
	}
	if s := strings.TrimSpace(os.Getenv("KAFKA_CONSUMER_GROUP")); s != "" {
		c.Kafka.ConsumerGroup = s
	}
	if s := strings.TrimSpace(os.Getenv("RUN_MIGRATIONS_ON_STARTUP")); s != "" {
		c.Runtime.RunMigrationsOnStartup = strings.EqualFold(s, "true") || s == "1"
	}
	if s := strings.TrimSpace(os.Getenv("WS_ALLOWED_ORIGINS")); s != "" {
		for _, p := range strings.Split(s, ",") {
			if t := strings.TrimSpace(p); t != "" {
				c.WS.AllowedOrigins = append(c.WS.AllowedOrigins, t)
			}
		}
	}
	if s := strings.TrimSpace(os.Getenv("SERVICE_NAME")); s != "" {
		c.Observability.ServiceName = s
	}
	if s := strings.TrimSpace(os.Getenv("APP_ENV")); s != "" {
		c.Observability.Env = s
	}
	if s := strings.TrimSpace(os.Getenv("LOG_FORMAT")); s != "" {
		c.Observability.LogFormat = s
	}
}

func (c *Config) applyObservabilityDefaults() {
	if c == nil {
		return
	}
	if strings.TrimSpace(c.Observability.ServiceName) == "" {
		c.Observability.ServiceName = "goflow-backend"
	}
	if strings.TrimSpace(c.Observability.Env) == "" {
		c.Observability.Env = "development"
	}
	if strings.TrimSpace(c.Observability.LogFormat) == "" {
		c.Observability.LogFormat = "text"
	}
}

func (c *Config) Validate() error {
	if c == nil {
		return errors.New("config: nil")
	}
	if strings.TrimSpace(c.App.Port) == "" {
		return errors.New("config: app.port is required (yaml or HTTP_PORT)")
	}
	if strings.TrimSpace(c.Postgres.DSN) == "" {
		return errors.New("config: postgres.dsn is required (yaml or POSTGRES_DSN)")
	}
	if strings.TrimSpace(c.Redis.Addr) == "" {
		return errors.New("config: redis.addr is required (yaml or REDIS_ADDR)")
	}
	if strings.TrimSpace(c.JWT.Secret) == "" {
		return errors.New("config: jwt.secret is required (yaml or JWT_SECRET)")
	}
	if len(strings.TrimSpace(c.JWT.Secret)) < 16 {
		return errors.New("config: jwt.secret must be at least 16 characters")
	}
	if c.JWT.AccessTTLSeconds <= 0 {
		return errors.New("config: jwt.access_ttl_seconds must be positive (yaml or JWT_ACCESS_TTL_SECONDS)")
	}
	if c.JWT.RefreshTTLSeconds <= 0 {
		return errors.New("config: jwt.refresh_ttl_seconds must be positive (yaml or JWT_REFRESH_TTL_SECONDS)")
	}
	if c.JWT.RefreshTTLSeconds < c.JWT.AccessTTLSeconds {
		return errors.New("config: jwt.refresh_ttl_seconds must be >= access_ttl_seconds")
	}
	if c.RateLimit.LoginPerMinute < 0 {
		return errors.New("config: rate_limit.login_per_minute must be >= 0")
	}
	if c.RateLimit.RegisterPerMinute < 0 {
		return errors.New("config: rate_limit.register_per_minute must be >= 0")
	}
	if c.RateLimit.MessageSendPerMinute < 0 {
		return errors.New("config: rate_limit.message_send_per_minute must be >= 0")
	}
	if c.RateLimit.WSConnectPerMinute < 0 {
		return errors.New("config: rate_limit.ws_connect_per_minute must be >= 0")
	}
	if c.Kafka.Enabled {
		if strings.TrimSpace(c.Kafka.Brokers) == "" {
			return errors.New("config: kafka.brokers is required when kafka.enabled is true")
		}
	}
	return nil
}
