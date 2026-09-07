//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// platformChangeRepo is deliberately small on top of the shared Gemini test
// repository.  The operation under test only needs to observe the account,
// persist the replacement, clear transient state, and bind the requested
// groups; all other repository methods remain harmless stubs.
type platformChangeRepo struct {
	mockAccountRepoForGemini
	account       *Account
	updateCalls   int
	boundAccount  int64
	boundGroupIDs []int64
	clearTemp     int
	clearRates    int
	shadows       []*Account
	updateErr     error
	clearTempErr  error
	clearRatesErr error
	bindErr       error
}

func (r *platformChangeRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	if r.account == nil || r.account.ID != id {
		return nil, ErrAccountNotFound
	}
	return r.account, nil
}

func (r *platformChangeRepo) Update(_ context.Context, account *Account) error {
	r.updateCalls++
	if r.updateErr != nil {
		return r.updateErr
	}
	r.account = account
	return nil
}

func (r *platformChangeRepo) ClearTempUnschedulable(_ context.Context, _ int64) error {
	r.clearTemp++
	return r.clearTempErr
}

func (r *platformChangeRepo) ClearModelRateLimits(_ context.Context, _ int64) error {
	r.clearRates++
	return r.clearRatesErr
}

func (r *platformChangeRepo) BindGroups(_ context.Context, id int64, groupIDs []int64) error {
	r.boundAccount = id
	r.boundGroupIDs = append([]int64(nil), groupIDs...)
	return r.bindErr
}

func (r *platformChangeRepo) ListShadowsByParent(_ context.Context, _ int64) ([]*Account, error) {
	return r.shadows, nil
}

type platformChangeCacheSpy struct{ calls int }

func (s *platformChangeCacheSpy) InvalidateCache() { s.calls++ }

func TestChangeAccountPlatform_ReplacesIdentityInPlaceAndResetsRuntime(t *testing.T) {
	accountID := int64(721)
	oldRateReset := time.Now().Add(time.Hour)
	oldSessionStart := time.Now().Add(-time.Hour)
	oldSessionEnd := time.Now().Add(time.Hour)
	note := "keep this note"
	repo := &platformChangeRepo{account: &Account{
		ID:                      accountID,
		Name:                    "shared account",
		Notes:                   &note,
		Platform:                PlatformAnthropic,
		Type:                    AccountTypeOAuth,
		Credentials:             map[string]any{"access_token": "old-anthropic-token"},
		Extra:                   map[string]any{"privacy_mode": "old", "old_provider_state": true},
		ProxyID:                 platformChangeInt64Ptr(88),
		Concurrency:             7,
		Priority:                3,
		Status:                  StatusError,
		ErrorMessage:            "old provider failed",
		Schedulable:             false,
		RateLimitedAt:           &oldSessionStart,
		RateLimitResetAt:        &oldRateReset,
		OverloadUntil:           &oldRateReset,
		TempUnschedulableUntil:  &oldRateReset,
		TempUnschedulableReason: "cooldown",
		SessionWindowStart:      &oldSessionStart,
		SessionWindowEnd:        &oldSessionEnd,
		SessionWindowStatus:     "active",
	}}
	groupRepo := &mockGroupRepoForGemini{groups: map[int64]*Group{42: {ID: 42}}}
	cache := &platformChangeCacheSpy{}
	svc := &adminServiceImpl{accountRepo: repo, groupRepo: groupRepo, channelCacheInvalidator: cache}
	groupIDs := []int64{42}

	updated, err := svc.ChangeAccountPlatform(context.Background(), accountID, &ChangeAccountPlatformInput{
		Platform:              PlatformOpenAI,
		Type:                  AccountTypeAPIKey,
		Credentials:           map[string]any{"api_key": "sk-new"},
		Extra:                 map[string]any{"privacy_mode": "should-not-carry", "model_mapping": map[string]any{"old": "new"}},
		GroupIDs:              &groupIDs,
		SkipMixedChannelCheck: true,
	})

	require.NoError(t, err)
	require.NotNil(t, updated)
	require.Equal(t, accountID, updated.ID)
	require.Equal(t, PlatformOpenAI, updated.Platform)
	require.Equal(t, AccountTypeAPIKey, updated.Type)
	require.Equal(t, "sk-new", updated.Credentials["api_key"])
	require.NotContains(t, updated.Credentials, "access_token")
	require.NotContains(t, updated.Extra, "privacy_mode")
	require.Contains(t, updated.Extra, "model_mapping")
	require.Equal(t, "shared account", updated.Name)
	require.Equal(t, note, *updated.Notes)
	require.Equal(t, int64(88), *updated.ProxyID)
	require.Equal(t, 7, updated.Concurrency)
	require.Equal(t, 3, updated.Priority)
	require.Equal(t, StatusActive, updated.Status)
	require.True(t, updated.Schedulable)
	require.Empty(t, updated.ErrorMessage)
	require.Nil(t, updated.RateLimitedAt)
	require.Nil(t, updated.RateLimitResetAt)
	require.Nil(t, updated.OverloadUntil)
	require.Nil(t, updated.TempUnschedulableUntil)
	require.Empty(t, updated.TempUnschedulableReason)
	require.Nil(t, updated.SessionWindowStart)
	require.Nil(t, updated.SessionWindowEnd)
	require.Empty(t, updated.SessionWindowStatus)
	require.Equal(t, groupIDs, updated.GroupIDs)
	require.Equal(t, 1, repo.updateCalls)
	require.Equal(t, 1, repo.clearTemp)
	require.Equal(t, 1, repo.clearRates)
	require.Equal(t, accountID, repo.boundAccount)
	require.Equal(t, groupIDs, repo.boundGroupIDs)
	require.Equal(t, 1, cache.calls)
}

func TestChangeAccountPlatform_RequiresExplicitCredentialsAndGroups(t *testing.T) {
	repo := &platformChangeRepo{account: &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey}}
	svc := &adminServiceImpl{accountRepo: repo}

	_, err := svc.ChangeAccountPlatform(context.Background(), 1, &ChangeAccountPlatformInput{
		Platform:    PlatformOpenAI,
		Credentials: map[string]any{"api_key": "new"},
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "group_ids must be provided")

	emptyGroups := []int64{}
	_, err = svc.ChangeAccountPlatform(context.Background(), 1, &ChangeAccountPlatformInput{
		Platform:    PlatformOpenAI,
		GroupIDs:    &emptyGroups,
		Credentials: map[string]any{},
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "credentials must be supplied")
	require.Zero(t, repo.updateCalls)
}

func TestChangeAccountPlatform_RejectsShadowAndShadowParent(t *testing.T) {
	parentID := int64(90)
	shadowRepo := &platformChangeRepo{account: &Account{ID: 91, ParentAccountID: &parentID}}
	emptyGroups := []int64{}
	svc := &adminServiceImpl{accountRepo: shadowRepo}
	_, err := svc.ChangeAccountPlatform(context.Background(), 91, &ChangeAccountPlatformInput{
		Platform: PlatformOpenAI, Credentials: map[string]any{"api_key": "new"}, GroupIDs: &emptyGroups,
	})
	require.ErrorContains(t, err, "spark shadow account")

	parentRepo := &platformChangeRepo{
		account: &Account{ID: parentID, Platform: PlatformAnthropic, Type: AccountTypeAPIKey},
		shadows: []*Account{{ID: 91, ParentAccountID: &parentID}},
	}
	svc = &adminServiceImpl{accountRepo: parentRepo}
	_, err = svc.ChangeAccountPlatform(context.Background(), parentID, &ChangeAccountPlatformInput{
		Platform: PlatformOpenAI, Credentials: map[string]any{"api_key": "new"}, GroupIDs: &emptyGroups,
	})
	require.ErrorContains(t, err, "has a spark shadow")
}

func TestPlatformChangeExtraClearsManagedStateButKeepsConfiguration(t *testing.T) {
	extra, err := platformChangeExtra(PlatformOpenAI, AccountTypeAPIKey, map[string]any{
		"privacy_mode":                        "provider-owned",
		"upstream_billing_probe":              true,
		"openai_long_context_billing_enabled": true,
		"model_mapping":                       map[string]any{"gpt-old": "gpt-new"},
	}, map[string]any{
		"model_mapping": map[string]any{"gpt-old": "gpt-overridden"},
	})
	require.NoError(t, err)
	require.NotContains(t, extra, "privacy_mode")
	require.NotContains(t, extra, "upstream_billing_probe")
	require.Equal(t, true, extra["openai_long_context_billing_enabled"])
	require.Equal(t, map[string]any{"gpt-old": "gpt-overridden"}, extra["model_mapping"])
}

func TestChangeAccountPlatform_PropagatesRepositoryCleanupError(t *testing.T) {
	repo := &platformChangeRepo{
		account:       &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey},
		clearRatesErr: errors.New("rate-limit cleanup failed"),
	}
	emptyGroups := []int64{}
	svc := &adminServiceImpl{accountRepo: repo}
	_, err := svc.ChangeAccountPlatform(context.Background(), 1, &ChangeAccountPlatformInput{
		Platform: PlatformOpenAI, Credentials: map[string]any{"api_key": "new"}, GroupIDs: &emptyGroups,
	})
	require.ErrorContains(t, err, "rate-limit cleanup failed")
	require.Equal(t, 1, repo.updateCalls)
}

func TestChangeAccountPlatform_RejectsAPIKeyWhenGroupRequiresOAuthOnly(t *testing.T) {
	repo := &platformChangeRepo{account: &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeOAuth}}
	groupRepo := &mockGroupRepoForGemini{groups: map[int64]*Group{
		55: {ID: 55, Name: "oauth-strict", Platform: PlatformOpenAI, RequireOAuthOnly: true},
	}}
	svc := &adminServiceImpl{accountRepo: repo, groupRepo: groupRepo}
	groupIDs := []int64{55}

	_, err := svc.ChangeAccountPlatform(context.Background(), 1, &ChangeAccountPlatformInput{
		Platform:              PlatformOpenAI,
		Type:                  AccountTypeAPIKey,
		Credentials:           map[string]any{"api_key": "sk-new"},
		GroupIDs:              &groupIDs,
		SkipMixedChannelCheck: true,
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "仅允许 OAuth 账号")
	require.Zero(t, repo.updateCalls)
}

func platformChangeInt64Ptr(v int64) *int64 { return &v }
