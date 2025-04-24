package logging

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

type LoggingConfig struct {
	Level      string
	FilePath   string
	MaxSize    int // megabytes
	MaxBackups int
	MaxAge     int // days
}

func InitLogger(cfg LoggingConfig) (*zap.Logger, error) {
	// Encoder for console (pretty)
	consoleEncoder := zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig())
	// Encoder for file (JSON)
	fileEncoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())

	// Log level
	var level zapcore.Level
	switch cfg.Level {
	case "debug":
		level = zapcore.DebugLevel
	case "info":
		level = zapcore.InfoLevel
	case "warn":
		level = zapcore.WarnLevel
	case "error":
		level = zapcore.ErrorLevel
	default:
		level = zapcore.InfoLevel
	}

	// File writer with rotation
	fileWriter := zapcore.AddSync(&lumberjack.Logger{
		Filename:   cfg.FilePath,
		MaxSize:    cfg.MaxSize,
		MaxBackups: cfg.MaxBackups,
		MaxAge:     cfg.MaxAge,
		Compress:   true,
	})

	// Console writer
	consoleWriter := zapcore.Lock(os.Stdout)

	core := zapcore.NewTee(
		zapcore.NewCore(consoleEncoder, consoleWriter, level),
		zapcore.NewCore(fileEncoder, fileWriter, level),
	)

	logger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
	return logger, nil
}

// WithRequestID returns a logger with the request ID field attached
func WithRequestID(logger *zap.Logger, requestID string) *zap.Logger {
	return logger.With(zap.String("request_id", requestID))
}
