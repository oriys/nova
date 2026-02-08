package main

import "os"

// Config holds Atlas configuration from environment variables.
type Config struct {
	URL       string
	APIKey    string
	Tenant    string
	Namespace string
}

func LoadConfig() *Config {
	url := os.Getenv("NOVA_URL")
	if url == "" {
		url = "http://localhost:9000"
	}
	return &Config{
		URL:       url,
		APIKey:    os.Getenv("NOVA_API_KEY"),
		Tenant:    os.Getenv("NOVA_TENANT"),
		Namespace: os.Getenv("NOVA_NAMESPACE"),
	}
}
