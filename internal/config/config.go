package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// SpaceConfig defines a space-to-directory mapping.
type SpaceConfig struct {
	Key              string `mapstructure:"key"`
	Path             string `mapstructure:"path"`
	ParentTitle      string `mapstructure:"parentTitle"`
	AutoCreateParent bool   `mapstructure:"autoCreateParent"`
	// ImageWidth is the default `ac:width` pixel value applied to images that
	// don't carry an explicit size hint in markdown. 0 selects the built-in
	// default (760, the typical Confluence content width).
	ImageWidth int `mapstructure:"imageWidth"`
}

// Config holds the full application configuration.
type Config struct {
	Email    string        `mapstructure:"email"`
	APIToken string        `mapstructure:"apiToken"`
	BaseURL  string        `mapstructure:"baseUrl"`
	Spaces   []SpaceConfig `mapstructure:"spaces"`
}

const (
	EnvPrefix     = "MD2CONFLUENCE"
	defaultDir    = ".config/md2confluence"
	defaultName   = "config"
	defaultFormat = "yaml"
)

// Load reads config via viper. The source order (highest wins) is:
//   - MD2CONFLUENCE_* environment variables (scalar fields only)
//   - The YAML config file
//
// If configPath is non-empty it is used verbatim; otherwise viper searches
// ~/.config/md2confluence/config.yaml. A missing config file is tolerated
// as long as env vars supply the required scalar fields.
func Load(configPath string) (*Config, error) {
	v := viper.New()
	v.SetConfigType(defaultFormat)

	v.SetEnvPrefix(EnvPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	// Explicitly bind camelCase keys so MD2CONFLUENCE_API_TOKEN maps to apiToken, etc.
	_ = v.BindEnv("email", EnvPrefix+"_EMAIL")
	_ = v.BindEnv("apiToken", EnvPrefix+"_API_TOKEN")
	_ = v.BindEnv("baseUrl", EnvPrefix+"_BASE_URL")

	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("get home dir: %w", err)
		}
		v.AddConfigPath(filepath.Join(home, defaultDir))
		v.SetConfigName(defaultName)
	}

	if err := v.ReadInConfig(); err != nil {
		if _, ok := errors.AsType[viper.ConfigFileNotFoundError](err); !ok {
			return nil, fmt.Errorf("read config: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if cfg.Email == "" || cfg.APIToken == "" || cfg.BaseURL == "" {
		return nil, fmt.Errorf("config must have email, apiToken, and baseUrl set (via config file or %s_EMAIL / %s_API_TOKEN / %s_BASE_URL env vars)", EnvPrefix, EnvPrefix, EnvPrefix)
	}

	if len(cfg.Spaces) == 0 {
		return nil, fmt.Errorf("config must define at least one space mapping")
	}

	return &cfg, nil
}
