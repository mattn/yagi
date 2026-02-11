package main

import (
	"github.com/yagi-agent/yagi/provider"
)

type Provider = provider.Provider

var providers = defaultProviders

var defaultProviders = provider.DefaultProviders

func findProvider(name string) *Provider {
	return provider.Find(name, providers)
}

func loadExtraProviders(configDir string) error {
	result, err := provider.LoadExtra(configDir)
	if err != nil {
		return err
	}
	providers = result
	return nil
}
