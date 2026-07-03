package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/thediveo/enumflag/v2"
)

// Map zerolog levels to their textual representations
var LogLevelIds = map[zerolog.Level][]string{
	zerolog.PanicLevel: {"panic"},
	zerolog.FatalLevel: {"fatal"},
	zerolog.ErrorLevel: {"error"},
	zerolog.WarnLevel:  {"warn", "warning"},
	zerolog.InfoLevel:  {"info"},
	zerolog.DebugLevel: {"debug"},
	zerolog.TraceLevel: {"trace"},
}

// Global log level variable with default
var logLevel zerolog.Level = zerolog.InfoLevel

var rootCmd = &cobra.Command{
	Use:   "cbzconverter",
	Short: "Convert CBZ files using a specified converter",
}

func SetVersionInfo(version, commit, date string) {
	rootCmd.Version = fmt.Sprintf("%s (Built on %s from Git SHA %s)", version, date, commit)
}

func getPath() string {
	return filepath.Join(map[string]string{
		"windows": filepath.Join(os.Getenv("APPDATA")),
		"darwin":  filepath.Join(os.Getenv("HOME"), ".config"),
		"linux":   filepath.Join(os.Getenv("HOME"), ".config"),
	}[runtime.GOOS], "CBZOptimizer")
}

func init() {
	configFolder := getPath()

	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(configFolder)
	viper.SetEnvPrefix("CBZ")
	viper.AutomaticEnv()

	// Add log level flag (accepts zerolog levels: panic, fatal, error, warn, info, debug, trace)
	ef := enumflag.New(&logLevel, "log", LogLevelIds, enumflag.EnumCaseInsensitive)
	rootCmd.PersistentFlags().VarP(
		ef,
		"log", "l",
		"Set log level; can be 'panic', 'fatal', 'error', 'warn', 'info', 'debug', or 'trace'")
	if err := ef.RegisterCompletion(rootCmd, "log", enumflag.Help[zerolog.Level]{
		zerolog.PanicLevel: "Only log panic messages",
		zerolog.FatalLevel: "Log fatal and panic messages",
		zerolog.ErrorLevel: "Log error, fatal, and panic messages",
		zerolog.WarnLevel:  "Log warn, error, fatal, and panic messages",
		zerolog.InfoLevel:  "Log info, warn, error, fatal, and panic messages",
		zerolog.DebugLevel: "Log debug, info, warn, error, fatal, and panic messages",
		zerolog.TraceLevel: "Log all messages including trace",
	}); err != nil {
		panic(fmt.Errorf("failed to register log completion: %w", err))
	}

	// Add log level environment variable support
	if err := viper.BindEnv("log", "LOG_LEVEL"); err != nil {
		panic(fmt.Errorf("failed to bind LOG_LEVEL env: %w", err))
	}
	if err := viper.BindPFlag("log", rootCmd.PersistentFlags().Lookup("log")); err != nil {
		panic(fmt.Errorf("failed to bind log flag: %w", err))
	}

	rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		ConfigureLogging()
	}

	// Ensure the configuration directory exists

	err := os.MkdirAll(configFolder, os.ModePerm)
	if err != nil {
		panic(fmt.Errorf("fatal error config file: %w", err))
	}
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			err := viper.SafeWriteConfig()
			if err != nil {
				panic(fmt.Errorf("fatal error config file: %w", err))
			}
		} else {
			panic(fmt.Errorf("fatal error config file: %w", err))
		}
	}
}

// Execute executes the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatal().Err(err).Msg("Command execution failed")
	}
}
func AddCommand(cmd *cobra.Command) {
	rootCmd.AddCommand(cmd)
}

// ConfigureLogging sets up zerolog based on command-line flags and environment variables
func ConfigureLogging() {
	// Start with default log level (info)
	level := zerolog.InfoLevel

	// Check LOG_LEVEL environment variable first
	envLogLevel := viper.GetString("log")
	if envLogLevel != "" {
		if parsedLevel, err := zerolog.ParseLevel(envLogLevel); err == nil {
			level = parsedLevel
		}
	}

	// Command-line log flag takes precedence over environment variable
	// The logLevel variable will be set by the flag parsing, so if it's different from default, use it
	if logLevel != zerolog.InfoLevel {
		level = logLevel
	}

	// Set the global log level
	zerolog.SetGlobalLevel(level)

	// Configure console writer for readable output
	log.Logger = log.Output(zerolog.ConsoleWriter{
		Out:     os.Stderr,
		NoColor: false,
	})
}
