package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/goccy/go-yaml"
)

type Provider struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
	Key  string `yaml:"key"`
}

type Config struct {
	Model            string `yaml:"model"`
	ProviderName     string
	BaseURL          string
	APIKey           string
	MaxTokens        int    `yaml:"max_tokens"`
	MaxTurns         int    `yaml:"max_turns"`
	MaxContextTokens int    `yaml:"max_context_tokens"`
	SystemPrompt     string `yaml:"system_prompt"`
}

func Load(configPath, providersPath string) (*Config, error) {
	providers, err := loadProviders(providersPath)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	providerName, modelName, ok := strings.Cut(cfg.Model, "/")
	if !ok {
		return nil, fmt.Errorf("model must be in <provider>/<model> format, got %q", cfg.Model)
	}

	provider, ok := providers[providerName]
	if !ok {
		return nil, fmt.Errorf("provider %q not found in %s", providerName, providersPath)
	}

	cfg.BaseURL = provider.URL
	cfg.APIKey = provider.Key
	cfg.ProviderName = providerName
	cfg.Model = modelName

	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = 4096
	}
	if cfg.MaxTurns == 0 {
		cfg.MaxTurns = 50
	}

	return &cfg, nil
}

func loadProviders(path string) (map[string]Provider, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read providers: %w", err)
	}

	var list []Provider
	if err := yaml.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("parse providers: %w", err)
	}

	m := make(map[string]Provider, len(list))
	for _, p := range list {
		m[p.Name] = p
	}

	for name, p := range m {
		p.URL, err = expandEnv(p.URL)
		if err != nil {
			return nil, fmt.Errorf("provider %q url: %w", name, err)
		}
		p.Key, err = expandEnv(p.Key)
		if err != nil {
			return nil, fmt.Errorf("provider %q key: %w", name, err)
		}
		m[name] = p
	}

	return m, nil
}

// expandEnv replaces ${VAR} patterns in s with the corresponding
// environment variable value. Returns an error if a referenced variable
// is not set.
func expandEnv(s string) (string, error) {
	var buf strings.Builder
	i := 0
	for i < len(s) {
		if i+1 < len(s) && s[i] == '$' && s[i+1] == '{' {
			end := strings.Index(s[i:], "}")
			if end == -1 {
				return "", fmt.Errorf("unclosed ${ in %q", s)
			}
			name := s[i+2 : i+end]
			val, ok := os.LookupEnv(name)
			if !ok {
				return "", fmt.Errorf("environment variable %q not set", name)
			}
			buf.WriteString(val)
			i += end + 1
		} else {
			buf.WriteByte(s[i])
			i++
		}
	}
	return buf.String(), nil
}
