package service

import (
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
)

const (
	envAutoFingerprint     = "SUB2API_AUTO_FINGERPRINT"
	envFingerprintHosts    = "SUB2API_FINGERPRINT_HOSTS"
	defaultFingerprintHost = "agentrouter.org"
)

// isAutoFingerprintEnabled 检查是否启用了自动客户端指纹注入。
// 默认开启，设置 false/0/off/no/disabled 可关闭。
func isAutoFingerprintEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(envAutoFingerprint)))
	switch v {
	case "false", "0", "off", "no", "disabled":
		return false
	default:
		return true
	}
}

// extractHostFromURL 从 URL 提取小写的 hostname（去除端口和路径）。
func extractHostFromURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(u.Hostname()))
}

// matchesFingerprintHost 判断目标 URL 的 host 是否命中指纹注入规则。
// 环境变量 SUB2API_FINGERPRINT_HOSTS 支持逗号分隔的域名列表；默认 agentrouter.org。
// 支持通配符 "*" 匹配所有 host，以及子域名匹配（如 api.agentrouter.org 匹配 agentrouter.org）。
func matchesFingerprintHost(rawURL string) bool {
	hostsEnv := strings.TrimSpace(os.Getenv(envFingerprintHosts))
	if hostsEnv == "" {
		hostsEnv = defaultFingerprintHost
	}

	patterns := strings.Split(hostsEnv, ",")
	for _, p := range patterns {
		pattern := strings.ToLower(strings.TrimSpace(p))
		if pattern == "*" {
			return true
		}
	}

	host := extractHostFromURL(rawURL)
	if host == "" {
		return false
	}

	for _, p := range patterns {
		pattern := strings.ToLower(strings.TrimSpace(p))
		if pattern == "" {
			continue
		}
		pattern = strings.TrimPrefix(pattern, "*.")
		pattern = strings.TrimPrefix(pattern, ".")
		if host == pattern || strings.HasSuffix(host, "."+pattern) {
			return true
		}
	}

	return false
}

// isRecognizedOfficialCLIUA 判断 User-Agent 是否已为官方 CLI 格式。
func isRecognizedOfficialCLIUA(ua string) bool {
	lower := strings.ToLower(strings.TrimSpace(ua))
	if lower == "" {
		return false
	}
	return strings.HasPrefix(lower, "codex_cli_rs/") ||
		strings.HasPrefix(lower, "codex-tui/") ||
		strings.HasPrefix(lower, "codex-vscode/") ||
		strings.HasPrefix(lower, "codex/") ||
		strings.HasPrefix(lower, "claude-cli/")
}

func getHeaderCaseInsensitive(h http.Header, name string) string {
	if h == nil {
		return ""
	}
	for k, vals := range h {
		if strings.EqualFold(k, name) && len(vals) > 0 {
			return vals[0]
		}
	}
	return ""
}

func setHeaderCaseInsensitive(h http.Header, name, value string) {
	if h == nil {
		return
	}
	for k := range h {
		if strings.EqualFold(k, name) {
			delete(h, k)
		}
	}
	h[resolveWireCasing(name)] = []string{value}
}

// applyCustomClientFingerprints 对需要过客户端指纹校验的上游（如 AgentRouter）自动注入客户端指纹。
//
// 契约与优先级：
// 1. 仅针对 API Key 账号（OAuth 账号已有内置的官方身份收敛机制）；
// 2. 本函数在 ApplyHeaderOverrides 开头调用，作为底色/兜底；
// 3. 用户在账号后台若显式配置了请求头覆写（header_overrides），其值会覆盖本函数注入的内容。
func applyCustomClientFingerprints(a *Account, h http.Header) {
	if a == nil || h == nil {
		return
	}
	if a.Type != AccountTypeAPIKey {
		return
	}
	if !isAutoFingerprintEnabled() {
		return
	}

	baseURL := strings.TrimSpace(a.GetCredential("base_url"))
	if baseURL == "" {
		baseURL = a.GetBaseURL()
	}
	if !matchesFingerprintHost(baseURL) {
		return
	}

	if a.IsAnthropic() || a.Platform == PlatformAnthropic {
		// Anthropic 平台指纹
		ua := getHeaderCaseInsensitive(h, "User-Agent")
		if ua == "" || !strings.HasPrefix(strings.ToLower(ua), "claude-cli/") {
			setHeaderCaseInsensitive(h, "User-Agent", "claude-cli/"+claude.CLIVersion()+" (external, cli)")
		}
		if getHeaderCaseInsensitive(h, "anthropic-version") == "" {
			setHeaderCaseInsensitive(h, "anthropic-version", "2023-06-01")
		}
		if getHeaderCaseInsensitive(h, "x-app") == "" {
			setHeaderCaseInsensitive(h, "x-app", "cli")
		}
	} else if a.IsOpenAICompatible() {
		// OpenAI 兼容平台指纹（AgentRouter OpenAI 协议）
		// 清除 X-Stainless-* 请求头，防止泄漏 Python/Node 等非官方客户端特征
		for k := range h {
			if strings.HasPrefix(strings.ToLower(k), "x-stainless-") {
				delete(h, k)
			}
		}

		ua := getHeaderCaseInsensitive(h, "User-Agent")
		if !isRecognizedOfficialCLIUA(ua) {
			setHeaderCaseInsensitive(h, "User-Agent", CodexCanonicalUserAgent())
		}
		if getHeaderCaseInsensitive(h, "originator") == "" {
			setHeaderCaseInsensitive(h, "originator", "codex_cli_rs")
		}
		if getHeaderCaseInsensitive(h, "version") == "" {
			setHeaderCaseInsensitive(h, "version", CodexCanonicalClientVersion())
		}
	} else {
		// 其它平台兜底：消除 Go 默认 User-Agent
		ua := getHeaderCaseInsensitive(h, "User-Agent")
		if ua == "" || strings.HasPrefix(strings.ToLower(ua), "go-http-client") {
			setHeaderCaseInsensitive(h, "User-Agent", CodexCanonicalUserAgent())
		}
	}
}

