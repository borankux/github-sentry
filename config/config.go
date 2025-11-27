package config

import (
	"errors"

	"github.com/spf13/viper"
)

type DatabaseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
	SSLMode  string `mapstructure:"sslmode"`
}

type FeishuConfig struct {
	WebhookURL    string `mapstructure:"webhook_url"`
	WebhookSecret string `mapstructure:"webhook_secret"`
}

type CommandsConfig struct {
	Organization string   `mapstructure:"organization"`
	Repo         string   `mapstructure:"repo"`
	Sequential   []string `mapstructure:"sequential"`
	Async        []string `mapstructure:"async"`
}

type Config struct {
	GitHubWebhookSecret string                    `mapstructure:"github_webhook_secret"`
	Addr                string                     `mapstructure:"addr"`
	StagingBranch       string                     `mapstructure:"staging_branch"`
	ScriptsFolder       string                     `mapstructure:"scripts_folder"` // Deprecated: use commands instead
	LogFolder           string                     `mapstructure:"log_folder"`
	Commands            map[string]CommandsConfig  `mapstructure:"commands"`
	Database            DatabaseConfig              `mapstructure:"database"`
	Feishu              FeishuConfig                `mapstructure:"feishu"`
}

func LoadConfig() (*Config, error) {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.SetDefault("addr", ":8080")

	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	if cfg.GitHubWebhookSecret == "" {
		return nil, errors.New("github_webhook_secret must be set in config.yml")
	}

	if cfg.StagingBranch == "" {
		return nil, errors.New("staging_branch must be set in config.yml")
	}

	if cfg.LogFolder == "" {
		return nil, errors.New("log_folder must be set in config.yml")
	}

	// Validate commands configuration
	// Check if any project has commands configured and validate organization/repo fields
	hasCommands := false
	if cfg.Commands != nil {
		for projectName, projectCommands := range cfg.Commands {
			if projectCommands.Organization == "" {
				return nil, errors.New("commands." + projectName + ".organization must be set in config.yml")
			}
			if projectCommands.Repo == "" {
				return nil, errors.New("commands." + projectName + ".repo must be set in config.yml")
			}
			if len(projectCommands.Sequential) > 0 || len(projectCommands.Async) > 0 {
				hasCommands = true
			}
		}
	}
	if !hasCommands && cfg.ScriptsFolder == "" {
		return nil, errors.New("either commands with project-specific configuration or scripts_folder must be set in config.yml")
	}

	if cfg.Database.Host == "" {
		return nil, errors.New("database.host must be set in config.yml")
	}

	if cfg.Database.DBName == "" {
		return nil, errors.New("database.dbname must be set in config.yml")
	}

	if cfg.Feishu.WebhookURL == "" {
		return nil, errors.New("feishu.webhook_url must be set in config.yml")
	}
	// WebhookSecret is optional - only required if using custom bot with signature

	// Set defaults
	if cfg.Database.Port == 0 {
		cfg.Database.Port = 5432
	}

	if cfg.Database.SSLMode == "" {
		cfg.Database.SSLMode = "disable"
	}

	return &cfg, nil
}
