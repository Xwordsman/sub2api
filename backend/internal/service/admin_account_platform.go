package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// platformChangeManagedExtraKeys are observations owned by the old provider
// or by the scheduler.  A platform change starts a fresh provider identity, so
// these values must not be carried over even when the caller supplies an Extra
// object.  User configuration (model_mapping, quota limits, etc.) remains
// available when explicitly included in the request.
var platformChangeManagedExtraKeys = map[string]struct{}{
	"privacy_mode":                           {},
	"model_rate_limits":                      {},
	"antigravity_quota_scopes":               {},
	"antigravity_credits_overages":           {},
	"antigravity_force_token_refresh":        {},
	"antigravity_force_token_refresh_at":     {},
	"antigravity_force_token_refresh_reason": {},
	"upstream_billing_probe_enabled":         {},
	"upstream_billing_rate_sync_enabled":     {},
	"upstream_billing_probe":                 {},
	"ollama_cloud_usage_session":             {},
	"ollama_cloud_usage_auto_refresh":        {},
	"ollama_cloud_usage_snapshot":            {},
	"codex_auto_reset_credit_state":          {},
}

var supportedAccountPlatformsForChange = map[string]struct{}{
	PlatformAnthropic:   {},
	PlatformOpenAI:      {},
	PlatformGemini:      {},
	PlatformAntigravity: {},
	PlatformGrok:        {},
	PlatformKimi:        {},
	PlatformZhipu:       {},
	PlatformDeepseek:    {},
}

var supportedAccountTypesForChange = map[string]struct{}{
	AccountTypeOAuth:          {},
	AccountTypeSetupToken:     {},
	AccountTypeAPIKey:         {},
	AccountTypeUpstream:       {},
	AccountTypeBedrock:        {},
	AccountTypeServiceAccount: {},
}

// ChangeAccountPlatform changes provider identity in place while preserving
// the account row and its general scheduling settings.  It deliberately does
// not call UpdateAccount: that method merges credentials and managed Extra
// fields according to the *old* platform.
func (s *adminServiceImpl) ChangeAccountPlatform(ctx context.Context, id int64, input *ChangeAccountPlatformInput) (*Account, error) {
	if input == nil {
		return nil, ErrAccountNilInput
	}
	if s == nil || s.accountRepo == nil {
		return nil, errors.New("account repository is not configured")
	}
	if input.GroupIDs == nil {
		return nil, infraerrors.BadRequest("ACCOUNT_PLATFORM_GROUPS_REQUIRED", "group_ids must be provided explicitly; use an empty array to clear groups")
	}

	platform := strings.ToLower(strings.TrimSpace(input.Platform))
	if _, ok := supportedAccountPlatformsForChange[platform]; !ok {
		return nil, infraerrors.Newf(http.StatusBadRequest, "ACCOUNT_PLATFORM_UNSUPPORTED", "unsupported account platform: %s", platform)
	}

	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, ErrAccountNotFound
	}
	if account.IsCredentialShadow() {
		return nil, infraerrors.New(http.StatusBadRequest, "SPARK_SHADOW_PLATFORM_IMMUTABLE", "cannot change the platform of a spark shadow account; change the parent account instead")
	}

	targetType := strings.ToLower(strings.TrimSpace(input.Type))
	if targetType == "" {
		targetType = account.Type
	}
	if _, ok := supportedAccountTypesForChange[targetType]; !ok {
		return nil, infraerrors.Newf(http.StatusBadRequest, "ACCOUNT_PLATFORM_TYPE_UNSUPPORTED", "unsupported account type: %s", targetType)
	}
	if targetType == AccountTypeBedrock && platform != PlatformAnthropic {
		return nil, infraerrors.BadRequest("ACCOUNT_PLATFORM_TYPE_INVALID", "bedrock accounts must use the anthropic platform")
	}
	if targetType == AccountTypeServiceAccount && platform != PlatformGemini && platform != PlatformAnthropic {
		return nil, infraerrors.BadRequest("ACCOUNT_PLATFORM_TYPE_INVALID", "service_account accounts must use the gemini or anthropic platform")
	}
	if len(input.Credentials) == 0 {
		return nil, infraerrors.BadRequest("ACCOUNT_PLATFORM_CREDENTIALS_REQUIRED", "new provider credentials must be supplied explicitly")
	}

	platformChanged := account.Platform != platform || account.Type != targetType
	if platformChanged {
		shadows, shadowErr := s.accountRepo.ListShadowsByParent(ctx, id)
		if shadowErr != nil {
			return nil, shadowErr
		}
		if len(shadows) > 0 {
			return nil, infraerrors.New(http.StatusBadRequest, "SPARK_SHADOW_PARENT_PLATFORM_IMMUTABLE", "cannot change platform or type while the account has a spark shadow; delete the shadow first")
		}
	}

	credentials, err := cloneAccountJSONMap(input.Credentials)
	if err != nil {
		return nil, fmt.Errorf("clone target credentials: %w", err)
	}
	if err := NormalizeHeaderOverrideCredentials(credentials); err != nil {
		return nil, err
	}
	// Never merge the old provider's secrets into the new identity.
	credentials = SanitizeStoredCredentials(platform, credentials)
	if len(credentials) == 0 {
		return nil, infraerrors.BadRequest("ACCOUNT_PLATFORM_CREDENTIALS_REQUIRED", "new provider credentials must contain at least one supported value")
	}

	extra, err := platformChangeExtra(platform, targetType, account.Extra, input.Extra)
	if err != nil {
		return nil, err
	}

	groupIDs := append([]int64(nil), (*input.GroupIDs)...)
	if err := s.validateGroupIDsExist(ctx, groupIDs); err != nil {
		return nil, err
	}
	if len(groupIDs) > 0 && targetType == AccountTypeAPIKey && s.groupRepo != nil {
		for _, gid := range groupIDs {
			g, err := s.groupRepo.GetByID(ctx, gid)
			if err != nil {
				return nil, err
			}
			if g != nil && g.RequireOAuthOnly && groupSupportsOAuthOnlyFilter(g.Platform) {
				return nil, infraerrors.Newf(http.StatusBadRequest, "ACCOUNT_PLATFORM_OAUTH_ONLY_GROUP", "分组 [%s] 仅允许 OAuth 账号，apikey 类型账号无法加入", g.Name)
			}
		}
	}
	if len(groupIDs) > 0 && !input.SkipMixedChannelCheck {
		if err := s.checkMixedChannelRisk(ctx, account.ID, platform, groupIDs); err != nil {
			return nil, err
		}
	}

	previousStatus := account.Status
	account.Platform = platform
	account.Type = targetType
	account.Credentials = credentials
	account.Extra = extra
	account.GroupIDs = groupIDs

	// A provider identity change invalidates transient state.  Keep durable
	// usage history and generic scheduling settings, but make the new identity
	// immediately eligible when the old account was in an error state.
	account.RateLimitedAt = nil
	account.RateLimitResetAt = nil
	account.OverloadUntil = nil
	account.TempUnschedulableUntil = nil
	account.TempUnschedulableReason = ""
	account.SessionWindowStart = nil
	account.SessionWindowEnd = nil
	account.SessionWindowStatus = ""
	account.ErrorMessage = ""
	if previousStatus == StatusError {
		account.Status = StatusActive
		account.Schedulable = true
	}

	if atomicRepo, ok := s.accountRepo.(AccountPlatformRepository); ok {
		// Production repositories implement this capability with one Ent
		// transaction.  Test doubles and older repository implementations keep
		// using the compatibility sequence below.
		if err := atomicRepo.ChangePlatformInPlace(ctx, account, groupIDs); err != nil {
			return nil, err
		}
	} else {
		if err := s.accountRepo.Update(ctx, account); err != nil {
			return nil, err
		}
		// Update() persists the common runtime columns; these dedicated calls
		// cover fields maintained by independent SQL paths (notably temp
		// unschedulable and model-level cooldowns).
		if err := s.accountRepo.ClearRateLimit(ctx, id); err != nil {
			return nil, err
		}
		if err := s.accountRepo.ClearAntigravityQuotaScopes(ctx, id); err != nil {
			return nil, err
		}
		if err := s.accountRepo.ClearTempUnschedulable(ctx, id); err != nil {
			return nil, err
		}
		if err := s.accountRepo.ClearModelRateLimits(ctx, id); err != nil {
			return nil, err
		}
		if err := s.accountRepo.BindGroups(ctx, id, groupIDs); err != nil {
			return nil, err
		}
	}

	if platformChanged && s.channelCacheInvalidator != nil {
		s.channelCacheInvalidator.InvalidateCache()
	}
	if s.runtimeBlocker != nil {
		s.runtimeBlocker.ClearAccountSchedulingBlock(id)
	}
	updated, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func platformChangeExtra(platform, accountType string, previous, requested map[string]any) (map[string]any, error) {
	// A platform change is an in-place edit, so durable user options should
	// survive the provider swap.  Start from a sanitized copy of the old
	// options, then let the explicit request override them.  The sanitizer
	// removes provider observations and scheduler state before either map is
	// persisted; callers cannot smuggle those values back through Extra.
	extra, err := duplicateAccountExtra(previous)
	if err != nil {
		return nil, fmt.Errorf("clone existing extra: %w", err)
	}
	requestedExtra, err := duplicateAccountExtra(requested)
	if err != nil {
		return nil, fmt.Errorf("clone target extra: %w", err)
	}
	if len(requestedExtra) > 0 {
		if extra == nil {
			extra = make(map[string]any, len(requestedExtra))
		}
		for key, value := range requestedExtra {
			extra[key] = value
		}
	}
	for key := range platformChangeManagedExtraKeys {
		delete(extra, key)
	}

	extra, err = normalizeOpenAILongContextBillingExtra(platform, extra)
	if err != nil {
		return nil, err
	}
	extra, err = normalizeGrokMediaEligibilityExtra(platform, extra)
	if err != nil {
		return nil, err
	}
	extra, err = normalizeOpenAIAutoResetCreditExtra(platform, accountType, false, extra)
	if err != nil {
		return nil, err
	}
	extra = prepareCodexFingerprintExtraForCreate(platform, accountType, extra)
	if extra != nil {
		if err := ValidateQuotaResetConfig(extra); err != nil {
			return nil, err
		}
		ComputeQuotaResetAt(extra)
		NormalizeFixedQuotaWindows(extra)
	}
	return extra, nil
}
