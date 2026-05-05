package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

// Config — единая конфигурация сервиса.
// Мы используем Viper, потому что сервис должен одинаково хорошо работать:
// - локально с `.env`;
// - в Docker/K8s с переменными окружения.
type Config struct {
	HTTPAddr           string
	LogLevel           string
	ShutdownTimeout    time.Duration
	HTTPBodyLimitBytes int64
	Mongo              MongoConfig
}

// MongoConfig — параметры подключения к MongoDB.
type MongoConfig struct {
	URI            string
	DB             string
	ConnectTimeout time.Duration
	PingTimeout    time.Duration
}

func Load() (Config, error) {
	// `.env` — удобство для локальной разработки; в проде обычно переменных окружения достаточно.
	// Ошибку игнорируем: файл может отсутствовать.
	_ = godotenv.Load()

	v := viper.New()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	v.SetDefault("http.addr", ":8080")
	v.SetDefault("log.level", "info")
	v.SetDefault("shutdown.timeout", "10s")
	v.SetDefault("http.body_limit_bytes", int64(8*1024*1024))

	v.SetDefault("mongo.connect_timeout", "5s")
	v.SetDefault("mongo.ping_timeout", "2s")

	connectTimeout := mustDuration(v, "mongo.connect_timeout")
	if connectTimeout <= 0 {
		connectTimeout = 5 * time.Second
	}
	pingTimeout := mustDuration(v, "mongo.ping_timeout")
	if pingTimeout <= 0 {
		pingTimeout = 2 * time.Second
	}
	shutdownTimeout := mustDuration(v, "shutdown.timeout")
	if shutdownTimeout <= 0 {
		shutdownTimeout = 10 * time.Second
	}

	cfg := Config{
		HTTPAddr:           v.GetString("http.addr"),
		LogLevel:           v.GetString("log.level"),
		ShutdownTimeout:    shutdownTimeout,
		HTTPBodyLimitBytes: v.GetInt64("http.body_limit_bytes"),
		Mongo: MongoConfig{
			URI:            v.GetString("mongo.uri"),
			DB:             v.GetString("mongo.db"),
			ConnectTimeout: connectTimeout,
			PingTimeout:    pingTimeout,
		},
	}

	if strings.TrimSpace(cfg.Mongo.URI) == "" {
		return Config{}, fmt.Errorf("missing config: MONGO_URI")
	}
	if strings.TrimSpace(cfg.Mongo.DB) == "" {
		return Config{}, fmt.Errorf("missing config: MONGO_DB")
	}
	if cfg.HTTPBodyLimitBytes <= 0 {
		return Config{}, fmt.Errorf("invalid config: HTTP_BODY_LIMIT_BYTES must be > 0")
	}
	return cfg, nil
}

func mustDuration(v *viper.Viper, key string) time.Duration {
	d := v.GetDuration(key)
	if d > 0 {
		return d
	}
	// viper.GetDuration возвращает 0 при неверном формате, поэтому даём безопасный дефолт.
	// Ошибку валидации конфигов в бою лучше ловить раньше (через env tests), но здесь важно не паниковать.
	return 0
}

