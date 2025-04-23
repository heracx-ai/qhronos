package config

import (
	"fmt"
	"time"

	"github.com/feedloop/qhronos/internal/database"
	"github.com/spf13/viper"
)

type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	Database  DatabaseConfig  `mapstructure:"database"`
	Redis     RedisConfig     `mapstructure:"redis"`
	Auth      AuthConfig      `mapstructure:"auth"`
	HMAC      HMACConfig      `mapstructure:"hmac"`
	Scheduler SchedulerConfig `mapstructure:"scheduler"`
	Retention RetentionConfig `mapstructure:"retention"`
}

type ServerConfig struct {
	Port    int           `mapstructure:"port"`
	Timeout time.Duration `mapstructure:"timeout"`
}

type DatabaseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
	SSLMode  string `mapstructure:"sslmode"`
}

type RedisConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type AuthConfig struct {
	MasterToken string `mapstructure:"master_token"`
	JWTSecret   string `mapstructure:"jwt_secret"`
}

type HMACConfig struct {
	DefaultSecret string `mapstructure:"default_secret"`
}

type SchedulerConfig struct {
	LookAheadDuration time.Duration `mapstructure:"look_ahead_duration"`
	ExpansionInterval time.Duration `mapstructure:"expansion_interval"`
}

type RetentionConfig struct {
	Events     time.Duration `mapstructure:"events"`
	Occurrences time.Duration `mapstructure:"occurrences"`
	CleanupInterval time.Duration `mapstructure:"cleanup_interval"`
}

func Load() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./config")

	// Set default values
	viper.SetDefault("server.port", 8080)
	viper.SetDefault("server.timeout", "30s")
	viper.SetDefault("database.host", "localhost")
	viper.SetDefault("database.port", 5432)
	viper.SetDefault("database.user", "postgres")
	viper.SetDefault("database.password", "postgres")
	viper.SetDefault("database.dbname", "qhronos")
	viper.SetDefault("database.sslmode", "disable")
	viper.SetDefault("redis.host", "localhost")
	viper.SetDefault("redis.port", 6379)
	viper.SetDefault("redis.password", "")
	viper.SetDefault("redis.db", 0)
	viper.SetDefault("hmac.default_secret", "qhronos.io")
	viper.SetDefault("scheduler.look_ahead_duration", "24h")
	viper.SetDefault("scheduler.expansion_interval", "5m")
	viper.SetDefault("retention.events", "30d")
	viper.SetDefault("retention.occurrences", "7d")
	viper.SetDefault("retention.cleanup_interval", "1h")

	// Read environment variables
	viper.AutomaticEnv()

	// Read config file
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
	}

	config := &Config{}
	if err := viper.Unmarshal(config); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	return config, nil
}

// ToDBConfig converts DatabaseConfig to database.Config
func (c DatabaseConfig) ToDBConfig() database.Config {
	return database.Config{
		Host:     c.Host,
		Port:     c.Port,
		User:     c.User,
		Password: c.Password,
		DBName:   c.DBName,
		SSLMode:  c.SSLMode,
	}
} 