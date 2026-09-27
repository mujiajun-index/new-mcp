package systemone

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	ProviderTypeSafe     = "typesafe"
	ProviderOpenRouter   = "openrouter"
	DefaultTypeSafeURL   = "https://api.typesafe.ai"
	DefaultRouterURL     = "https://openrouter.ai/api/alpha/decisions"
	DefaultTypeSafeModel = "jev-latest"
	DefaultRouterModel   = "~typesafe/jev-latest"
)

// EndpointURL is a base URL for TypeSafe-compatible services and the full
// Decisions URL for OpenRouter-compatible services.
func ResolveEndpoint(provider, address string) (string, error) {
	if provider != ProviderTypeSafe && provider != ProviderOpenRouter {
		return "", fmt.Errorf("provider must be typesafe or openrouter")
	}
	if address == "" {
		if provider == ProviderTypeSafe {
			address = DefaultTypeSafeURL
		} else {
			address = DefaultRouterURL
		}
	}
	u, err := url.Parse(strings.TrimSpace(address))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.Fragment != "" || u.RawQuery != "" {
		return "", fmt.Errorf("endpoint_url must be an absolute http(s) URL without credentials, query or fragment")
	}
	if provider == ProviderTypeSafe {
		return u.JoinPath("v1", "systemone").String(), nil
	}
	return u.String(), nil
}

func DefaultModel(provider string) string {
	if provider == ProviderOpenRouter {
		return DefaultRouterModel
	}
	return DefaultTypeSafeModel
}

func NewClient(provider, address, key, model string) (*Client, error) {
	endpoint, err := ResolveEndpoint(provider, address)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(key) == "" {
		return nil, fmt.Errorf("api_key is required")
	}
	if strings.TrimSpace(model) == "" {
		model = DefaultModel(provider)
	}
	return &Client{URL: endpoint, APIKey: key, Model: model,
		HTTP: &http.Client{Timeout: 60 * time.Second}, Backoff: time.Second}, nil
}
