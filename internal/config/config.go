package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DataDir               string `json:"data_dir"`
	LogLevel              string `json:"log_level"`
	ControlAddress        string `json:"control_address"`
	PprofAddress          string `json:"pprof_address"`
	SQLiteBusyTimeoutMS   int    `json:"sqlite_busy_timeout_ms"`
	SQLiteMaxOpenConns    int    `json:"sqlite_max_open_conns"`
	MaxFrameBytes         int    `json:"max_frame_bytes"`
	MaxControlConnections int    `json:"max_control_connections"`
}

type Overrides struct {
	DataDir        *string
	LogLevel       *string
	ControlAddress *string
	PprofAddress   *string
}

func Defaults() Config {
	return Config{
		DataDir:               "./runtime-data",
		LogLevel:              "info",
		SQLiteBusyTimeoutMS:   5000,
		SQLiteMaxOpenConns:    8,
		MaxFrameBytes:         4 << 20,
		MaxControlConnections: 32,
	}
}

func Load(
	filePath string,
	overrides Overrides,
) (Config, error) {
	return LoadWithEnv(
		filePath,
		os.Getenv,
		overrides,
	)
}

func LoadWithEnv(
	filePath string,
	getenv func(string) string,
	overrides Overrides,
) (Config, error) {
	cfg := Defaults()

	if filePath != "" {
		file, err := os.Open(filePath)
		if err != nil {
			return Config{}, fmt.Errorf(
				"open config file: %w",
				err,
			)
		}

		defer file.Close()

		decoder := json.NewDecoder(file)
		decoder.DisallowUnknownFields()

		if err := decoder.Decode(&cfg); err != nil {
			return Config{}, fmt.Errorf(
				"decode config file: %w",
				err,
			)
		}

		if err := ensureEOF(decoder); err != nil {
			return Config{}, err
		}
	}

	if err := applyEnvironment(
		&cfg,
		getenv,
	); err != nil {
		return Config{}, err
	}

	applyOverrides(&cfg, overrides)

	if err := cfg.Normalize(); err != nil {
		return Config{}, err
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any

	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf(
				"decode config file: multiple JSON values",
			)
		}

		return fmt.Errorf(
			"decode config file: %w",
			err,
		)
	}

	return nil
}

func applyEnvironment(
	cfg *Config,
	getenv func(string) string,
) error {
	setString := func(
		key string,
		target *string,
	) {
		value := strings.TrimSpace(getenv(key))

		if value != "" {
			*target = value
		}
	}

	setInt := func(
		key string,
		target *int,
	) error {
		value := strings.TrimSpace(getenv(key))

		if value == "" {
			return nil
		}

		parsed, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf(
				"parse %s: %w",
				key,
				err,
			)
		}

		*target = parsed

		return nil
	}

	setString(
		"GO_AGENT_DATA_DIR",
		&cfg.DataDir,
	)

	setString(
		"GO_AGENT_LOG_LEVEL",
		&cfg.LogLevel,
	)

	setString(
		"GO_AGENT_CONTROL_ADDRESS",
		&cfg.ControlAddress,
	)

	setString(
		"GO_AGENT_PPROF_ADDRESS",
		&cfg.PprofAddress,
	)

	integers := []struct {
		key    string
		target *int
	}{
		{
			"GO_AGENT_SQLITE_BUSY_TIMEOUT_MS",
			&cfg.SQLiteBusyTimeoutMS,
		},
		{
			"GO_AGENT_SQLITE_MAX_OPEN_CONNS",
			&cfg.SQLiteMaxOpenConns,
		},
		{
			"GO_AGENT_MAX_FRAME_BYTES",
			&cfg.MaxFrameBytes,
		},
		{
			"GO_AGENT_MAX_CONTROL_CONNECTIONS",
			&cfg.MaxControlConnections,
		},
	}

	for _, item := range integers {
		if err := setInt(
			item.key,
			item.target,
		); err != nil {
			return err
		}
	}

	return nil
}

func applyOverrides(
	cfg *Config,
	overrides Overrides,
) {
	if overrides.DataDir != nil {
		cfg.DataDir = *overrides.DataDir
	}

	if overrides.LogLevel != nil {
		cfg.LogLevel = *overrides.LogLevel
	}

	if overrides.ControlAddress != nil {
		cfg.ControlAddress =
			*overrides.ControlAddress
	}

	if overrides.PprofAddress != nil {
		cfg.PprofAddress =
			*overrides.PprofAddress
	}
}

func (c *Config) Normalize() error {
	if strings.TrimSpace(c.DataDir) == "" {
		return fmt.Errorf(
			"data_dir must not be empty",
		)
	}

	clean, err := filepath.Abs(
		filepath.Clean(c.DataDir),
	)
	if err != nil {
		return fmt.Errorf(
			"normalize data_dir: %w",
			err,
		)
	}

	c.DataDir = clean

	if c.ControlAddress == "" {
		if runtime.GOOS == "windows" {
			c.ControlAddress =
				`\\.\pipe\go-agent`
		} else {
			c.ControlAddress = filepath.Join(
				c.DataDir,
				"control.sock",
			)
		}
	}

	return nil
}

func (c Config) Validate() error {
	switch strings.ToLower(c.LogLevel) {
	case "debug",
		"info",
		"warn",
		"warning",
		"error":
	default:
		return fmt.Errorf(
			"invalid log_level %q",
			c.LogLevel,
		)
	}

	if c.SQLiteBusyTimeoutMS < 1 ||
		c.SQLiteBusyTimeoutMS > 60_000 {
		return fmt.Errorf(
			"sqlite_busy_timeout_ms must be between 1 and 60000",
		)
	}

	if c.SQLiteMaxOpenConns < 1 ||
		c.SQLiteMaxOpenConns > 128 {
		return fmt.Errorf(
			"sqlite_max_open_conns must be between 1 and 128",
		)
	}

	if c.MaxFrameBytes < 1024 ||
		c.MaxFrameBytes > 16<<20 {
		return fmt.Errorf(
			"max_frame_bytes must be between 1024 and 16777216",
		)
	}

	if c.MaxControlConnections < 1 ||
		c.MaxControlConnections > 1024 {
		return fmt.Errorf(
			"max_control_connections must be between 1 and 1024",
		)
	}

	return nil
}

func (c Config) DatabasePath() string {
	return filepath.Join(
		c.DataDir,
		"runtime.db",
	)
}

func (c Config) ObjectStorePath() string {
	return filepath.Join(
		c.DataDir,
		"objects",
	)
}

func (c Config) SQLiteBusyTimeout() time.Duration {
	return time.Duration(
		c.SQLiteBusyTimeoutMS,
	) * time.Millisecond
}
