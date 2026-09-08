//go:build unit

package service

import (
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/stretchr/testify/require"
)

func TestAutoFingerprint_OpenAI_AgentRouterDefault(t *testing.T) {
	acc := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"base_url": "https://agentrouter.org/v1",
		},
	}

	h := http.Header{}
	h.Set("User-Agent", "Go-http-client/1.1")
	h.Set("X-Stainless-Lang", "python")
	h.Set("X-Stainless-Package-Version", "1.12.0")

	acc.ApplyHeaderOverrides(h)

	require.Equal(t, CodexCanonicalUserAgent(), getHeaderRaw(h, "User-Agent"))
	require.Equal(t, "codex_cli_rs", getHeaderRaw(h, "originator"))
	require.Equal(t, CodexCanonicalClientVersion(), getHeaderRaw(h, "version"))
	require.Empty(t, getHeaderRaw(h, "X-Stainless-Lang"))
	require.Empty(t, getHeaderRaw(h, "X-Stainless-Package-Version"))
}

func TestAutoFingerprint_Anthropic_AgentRouterDefault(t *testing.T) {
	acc := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"base_url": "https://agentrouter.org",
		},
	}

	h := http.Header{}
	h.Set("User-Agent", "curl/8.1.2")

	acc.ApplyHeaderOverrides(h)

	wantUA := "claude-cli/" + claude.CLIVersion() + " (external, cli)"
	require.Equal(t, wantUA, getHeaderRaw(h, "User-Agent"))
	require.Equal(t, "2023-06-01", getHeaderRaw(h, "anthropic-version"))
	require.Equal(t, "cli", getHeaderRaw(h, "x-app"))
}

func TestAutoFingerprint_PreservesExistingOfficialCLIUA(t *testing.T) {
	t.Run("preserves codex cli ua", func(t *testing.T) {
		acc := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Credentials: map[string]any{
				"base_url": "https://agentrouter.org/v1",
			},
		}
		h := http.Header{}
		existingUA := "codex_cli_rs/0.144.1 (Ubuntu 22.4.0; x86_64) xterm-256color"
		h.Set("User-Agent", existingUA)

		acc.ApplyHeaderOverrides(h)

		require.Equal(t, existingUA, getHeaderRaw(h, "User-Agent"))
		require.Equal(t, "codex_cli_rs", getHeaderRaw(h, "originator"))
		require.Equal(t, CodexCanonicalClientVersion(), getHeaderRaw(h, "version"))
	})

	t.Run("preserves claude cli ua", func(t *testing.T) {
		acc := &Account{
			Platform: PlatformAnthropic,
			Type:     AccountTypeAPIKey,
			Credentials: map[string]any{
				"base_url": "https://agentrouter.org",
			},
		}
		h := http.Header{}
		existingUA := "claude-cli/2.1.258 (external, cli)"
		h.Set("User-Agent", existingUA)

		acc.ApplyHeaderOverrides(h)

		require.Equal(t, existingUA, getHeaderRaw(h, "User-Agent"))
		require.Equal(t, "2023-06-01", getHeaderRaw(h, "anthropic-version"))
	})
}

func TestAutoFingerprint_ManualOverridePrecedence(t *testing.T) {
	acc := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"base_url":                   "https://agentrouter.org/v1",
			credKeyHeaderOverrideEnabled: true,
			credKeyHeaderOverrides: map[string]any{
				"user-agent": "custom-override-ua/9.9",
				"originator": "custom_originator",
				"version":    "1.0.0",
			},
		},
	}

	h := http.Header{}
	acc.ApplyHeaderOverrides(h)

	// Explicit overrides in account credentials MUST win
	require.Equal(t, "custom-override-ua/9.9", getHeaderRaw(h, "User-Agent"))
	require.Equal(t, "custom_originator", getHeaderRaw(h, "originator"))
	require.Equal(t, "1.0.0", getHeaderRaw(h, "version"))
}

func TestAutoFingerprint_DisabledViaEnv(t *testing.T) {
	t.Setenv("SUB2API_AUTO_FINGERPRINT", "false")

	acc := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"base_url": "https://agentrouter.org/v1",
		},
	}

	h := http.Header{}
	h.Set("User-Agent", "original-client/1.0")

	acc.ApplyHeaderOverrides(h)

	require.Equal(t, "original-client/1.0", getHeaderRaw(h, "User-Agent"))
	require.Empty(t, getHeaderRaw(h, "originator"))
	require.Empty(t, getHeaderRaw(h, "version"))
}

func TestAutoFingerprint_UnmatchedHost_NoOp(t *testing.T) {
	acc := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"base_url": "https://api.openai.com/v1",
		},
	}

	h := http.Header{}
	h.Set("User-Agent", "my-client/1.0")

	acc.ApplyHeaderOverrides(h)

	require.Equal(t, "my-client/1.0", getHeaderRaw(h, "User-Agent"))
	require.Empty(t, getHeaderRaw(h, "originator"))
}

func TestAutoFingerprint_CustomHostList(t *testing.T) {
	t.Setenv("SUB2API_FINGERPRINT_HOSTS", "custom-gateway.io, specialized-router.net")

	t.Run("matches configured host", func(t *testing.T) {
		acc := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Credentials: map[string]any{
				"base_url": "https://custom-gateway.io/v1",
			},
		}
		h := http.Header{}
		acc.ApplyHeaderOverrides(h)
		require.Equal(t, CodexCanonicalUserAgent(), getHeaderRaw(h, "User-Agent"))
		require.Equal(t, "codex_cli_rs", getHeaderRaw(h, "originator"))
	})

	t.Run("matches subdomain of configured host", func(t *testing.T) {
		acc := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Credentials: map[string]any{
				"base_url": "https://api.specialized-router.net/v1",
			},
		}
		h := http.Header{}
		acc.ApplyHeaderOverrides(h)
		require.Equal(t, CodexCanonicalUserAgent(), getHeaderRaw(h, "User-Agent"))
	})

	t.Run("does not match agentrouter when overridden", func(t *testing.T) {
		acc := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Credentials: map[string]any{
				"base_url": "https://agentrouter.org/v1",
			},
		}
		h := http.Header{}
		h.Set("User-Agent", "orig")
		acc.ApplyHeaderOverrides(h)
		require.Equal(t, "orig", getHeaderRaw(h, "User-Agent"))
	})
}

func TestAutoFingerprint_WildcardHost(t *testing.T) {
	t.Setenv("SUB2API_FINGERPRINT_HOSTS", "*")

	acc := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"base_url": "https://arbitrary-router.org/v1",
		},
	}

	h := http.Header{}
	acc.ApplyHeaderOverrides(h)

	require.Equal(t, CodexCanonicalUserAgent(), getHeaderRaw(h, "User-Agent"))
	require.Equal(t, "codex_cli_rs", getHeaderRaw(h, "originator"))
	require.Equal(t, CodexCanonicalClientVersion(), getHeaderRaw(h, "version"))
}

func TestAutoFingerprint_OAuthAccount_NoOp(t *testing.T) {
	acc := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"base_url": "https://agentrouter.org",
		},
	}

	h := http.Header{}
	h.Set("User-Agent", "oauth-original")

	acc.ApplyHeaderOverrides(h)

	require.Equal(t, "oauth-original", getHeaderRaw(h, "User-Agent"))
	require.Empty(t, getHeaderRaw(h, "anthropic-version"))
}
