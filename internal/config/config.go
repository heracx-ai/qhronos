package config

import (
	"fmt"
	"time"

	"github.com/feedloop/qhronos/internal/database"
	"github.com/spf13/viper"
)

type ArchivalConfig struct {
	CheckPeriod time.Duration `mapstructure:"check_period"`
}

type LoggingConfig struct {
	Level      string `mapstructure:"level"`
	FilePath   string `mapstructure:"file_path"`
	MaxSize    int    `mapstructure:"max_size"`
	MaxBackups int    `mapstructure:"max_backups"`
	MaxAge     int    `mapstructure:"max_age"`
}

type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	Database  DatabaseConfig  `mapstructure:"database"`
	Redis     RedisConfig     `mapstructure:"redis"`
	Auth      AuthConfig      `mapstructure:"auth"`
	HMAC      HMACConfig      `mapstructure:"hmac"`
	Scheduler SchedulerConfig `mapstructure:"scheduler"`
	Retention RetentionConfig `mapstructure:"retention"`
	Archival  ArchivalConfig  `mapstructure:"archival"`
	Logging   LoggingConfig   `mapstructure:"logging"`
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
	LookAheadDuration   time.Duration `mapstructure:"look_ahead_duration"`
	ExpansionInterval   time.Duration `mapstructure:"expansion_interval"`
	DispatchWorkerCount int           `mapstructure:"dispatch_worker_count"`
}

type RetentionConfig struct {
	Events          string `mapstructure:"events"`
	Occurrences     string `mapstructure:"occurrences"`
	CleanupInterval string `mapstructure:"cleanup_interval"`
}

type RetentionDurations struct {
	Events          time.Duration
	Occurrences     time.Duration
	CleanupInterval time.Duration
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
	viper.SetDefault("archival.check_period", "1h")
	viper.SetDefault("logging.level", "info")
	viper.SetDefault("logging.file_path", "qhronos.log")
	viper.SetDefault("logging.max_size", 10)
	viper.SetDefault("logging.max_backups", 3)
	viper.SetDefault("logging.max_age", 7)

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

// ParseRetentionDurations parses the string fields in RetentionConfig into time.Duration values.
func (c *Config) ParseRetentionDurations() (*RetentionDurations, error) {
	events, err := ParseFlexibleDuration(c.Retention.Events)
	if err != nil {
		return nil, fmt.Errorf("invalid retention.events: %w", err)
	}
	occurrences, err := ParseFlexibleDuration(c.Retention.Occurrences)
	if err != nil {
		return nil, fmt.Errorf("invalid retention.occurrences: %w", err)
	}
	cleanup, err := ParseFlexibleDuration(c.Retention.CleanupInterval)
	if err != nil {
		return nil, fmt.Errorf("invalid retention.cleanup_interval: %w", err)
	}
	return &RetentionDurations{
		Events:          events,
		Occurrences:     occurrences,
		CleanupInterval: cleanup,
	}, nil
}

// ParseFlexibleDuration parses a duration string supporting 'd' (days) and 'w' (weeks) in addition to standard units.
func ParseFlexibleDuration(s string) (time.Duration, error) {
	if len(s) == 0 {
		return 0, fmt.Errorf("empty duration string")
	}
	// Check for 'd' (days) and 'w' (weeks) at the end
	if len(s) > 1 {
		suffix := s[len(s)-1:]
		value := s[:len(s)-1]
		switch suffix {
		case "d":
			n, err := time.ParseDuration(value + "h")
			if err != nil {
				return 0, err
			}
			return n * 24, nil
		case "w":
			n, err := time.ParseDuration(value + "h")
			if err != nil {
				return 0, err
			}
			return n * 24 * 7, nil
		}
	}
	return time.ParseDuration(s)
}

// LoadWithPath loads configuration from a specific file path
func LoadWithPath(path string) (*Config, error) {
	if path == "" {
		return Load()
	}
	viper.SetConfigFile(path)
	viper.SetConfigType("yaml")

	// Set default values (same as in Load)
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
	viper.SetDefault("archival.check_period", "1h")
	viper.SetDefault("logging.level", "info")
	viper.SetDefault("logging.file_path", "qhronos.log")
	viper.SetDefault("logging.max_size", 10)
	viper.SetDefault("logging.max_backups", 3)
	viper.SetDefault("logging.max_age", 7)

	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}

	config := &Config{}
	if err := viper.Unmarshal(config); err != nil {
		return nil, err
	}

	return config, nil
}
