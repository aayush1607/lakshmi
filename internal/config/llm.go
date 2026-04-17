// Package config — Azure AI Foundry credentials loader.
//
// Kept in its own file because the surface is distinct from broker:
// env vars only, no disk persistence, no OS keychain. Users export
// these from their shell or a .env and Lakshmi reads them once at
// startup. Rotating a key just means re-launching the process.
package config

import (
	"errors"
	"fmt"
	"os"
)

// LLM holds the Azure Foundry connection parameters.
type LLM struct {
	Endpoint   string
	Deployment string
	APIKey     string
	APIVersion string // optional; defaults handled by the llm package
}

// ErrLLMNotConfigured is returned when required env vars are missing.
// Callers can use errors.Is to branch into "stub mode" where the REPL
// fallback explains what to set. We don't want to hard-fail at startup
// just because the user hasn't wired LLM yet — /portfolio etc. still work.
var ErrLLMNotConfigured = errors.New("azure foundry not configured")

// LoadLLM reads Foundry credentials from the environment. Returns
// ErrLLMNotConfigured (wrapped) when any required var is empty.
//
// Required:
//
//	AZURE_FOUNDRY_ENDPOINT     e.g. https://my-resource.openai.azure.com
//	AZURE_FOUNDRY_DEPLOYMENT   deployed model name (e.g. gpt-4o-mini)
//	AZURE_FOUNDRY_API_KEY      api key for the resource
//
// Optional:
//
//	AZURE_FOUNDRY_API_VERSION  defaults to the llm package default
func LoadLLM() (LLM, error) {
	l := LLM{
		Endpoint:   os.Getenv("AZURE_FOUNDRY_ENDPOINT"),
		Deployment: os.Getenv("AZURE_FOUNDRY_DEPLOYMENT"),
		APIKey:     os.Getenv("AZURE_FOUNDRY_API_KEY"),
		APIVersion: os.Getenv("AZURE_FOUNDRY_API_VERSION"),
	}
	var missing []string
	if l.Endpoint == "" {
		missing = append(missing, "AZURE_FOUNDRY_ENDPOINT")
	}
	if l.Deployment == "" {
		missing = append(missing, "AZURE_FOUNDRY_DEPLOYMENT")
	}
	if l.APIKey == "" {
		missing = append(missing, "AZURE_FOUNDRY_API_KEY")
	}
	if len(missing) > 0 {
		return l, fmt.Errorf("%w: missing %v", ErrLLMNotConfigured, missing)
	}
	return l, nil
}
