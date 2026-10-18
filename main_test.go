package main

import (
	"log/slog"
	"testing"

	"github.com/spf13/viper"
)

func TestParseLogLevel(t *testing.T) {
	tests := map[string]slog.Level{
		"debug":   slog.LevelDebug,
		"info":    slog.LevelInfo,
		"warn":    slog.LevelWarn,
		"error":   slog.LevelError,
		"":        slog.LevelInfo,
		"unknown": slog.LevelInfo,
	}
	for in, want := range tests {
		if got := parseLogLevel(in); got != want {
			t.Errorf("parseLogLevel(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestConfigureLogging(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("logging.level", "error")
	configureLogging()

	if slog.Default().Enabled(t.Context(), slog.LevelWarn) {
		t.Error("warn should be disabled at error level")
	}
	if !slog.Default().Enabled(t.Context(), slog.LevelError) {
		t.Error("error should be enabled at error level")
	}
}

func TestInitializeSentry_NoDSN(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	initializeSentry()
}

func TestInitializeSentry_InvalidDSN(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set(sentryDSNKey, "not-a-valid-dsn")
	initializeSentry()
}

func TestInitializeSentry_ValidDSN(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set(sentryDSNKey, "https://public@example.invalid/1")
	viper.Set(sentryEnvironmentKey, "test")
	viper.Set("sentry.debug", true)
	initializeSentry()
}
