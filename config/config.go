package config

import (
	"bytes"
	"fmt"
	"net/url"
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
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := strictUnmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	providerName, modelName, ok := strings.Cut(cfg.Model, "/")
	if !ok || providerName == "" || modelName == "" {
		return nil, fmt.Errorf("model must be in <provider>/<model> format, got %q", cfg.Model)
	}

	providers, err := loadProviders(providersPath)
	if err != nil {
		return nil, err
	}

	provider, ok := providers[providerName]
	if !ok {
		return nil, fmt.Errorf("provider %q not found in %s", providerName, providersPath)
	}

	provider, err = resolveProvider(providerName, provider)
	if err != nil {
		return nil, err
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

	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func loadProviders(path string) (map[string]Provider, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read providers: %w", err)
	}

	var list []Provider
	if err := strictUnmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("parse providers: %w", err)
	}

	m := make(map[string]Provider, len(list))
	for _, p := range list {
		if p.Name == "" {
			return nil, fmt.Errorf("provider name is required")
		}
		if _, exists := m[p.Name]; exists {
			return nil, fmt.Errorf("duplicate provider name %q", p.Name)
		}
		m[p.Name] = p
	}

	return m, nil
}

func resolveProvider(name string, p Provider) (Provider, error) {
	var err error
	p.URL, err = expandEnv(p.URL)
	if err != nil {
		return Provider{}, fmt.Errorf("provider %q url: %w", name, err)
	}
	p.Key, err = expandEnv(p.Key)
	if err != nil {
		return Provider{}, fmt.Errorf("provider %q key: %w", name, err)
	}
	if _, err := url.ParseRequestURI(p.URL); err != nil {
		return Provider{}, fmt.Errorf("provider %q url: %w", name, err)
	}

	return p, nil
}

func strictUnmarshal(data []byte, v any) error {
	dec := yaml.NewDecoder(bytes.NewReader(data), yaml.DisallowUnknownField())
	return dec.Decode(v)
}

func validateConfig(cfg Config) error {
	if cfg.Model == "" {
		return fmt.Errorf("config model is required")
	}
	if cfg.BaseURL == "" {
		return fmt.Errorf("config base URL is required")
	}
	if cfg.MaxTokens < 0 {
		return fmt.Errorf("config max_tokens must be non-negative")
	}
	if cfg.MaxTurns <= 0 {
		return fmt.Errorf("config max_turns must be greater than zero")
	}
	if cfg.MaxContextTokens < 0 {
		return fmt.Errorf("config max_context_tokens must be non-negative")
	}
	return nil
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
