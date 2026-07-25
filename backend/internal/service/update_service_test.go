//go:build unit

package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type updateServiceCacheStub struct {
	data string
}

func (s *updateServiceCacheStub) GetUpdateInfo(context.Context) (string, error) {
	if s.data == "" {
		return "", errors.New("cache miss")
	}
	return s.data, nil
}

func (s *updateServiceCacheStub) SetUpdateInfo(_ context.Context, data string, _ time.Duration) error {
	s.data = data
	return nil
}

type updateServiceGitHubClientStub struct {
	release        *GitHubRelease
	recentReleases []*GitHubRelease
	recentErr      error
	latestRepo     string
	recentRepo     string
}

func (s *updateServiceGitHubClientStub) FetchLatestRelease(_ context.Context, repo string) (*GitHubRelease, error) {
	s.latestRepo = repo
	return s.release, nil
}

func (s *updateServiceGitHubClientStub) FetchRecentReleases(_ context.Context, repo string, _ int) ([]*GitHubRelease, error) {
	s.recentRepo = repo
	return s.recentReleases, s.recentErr
}

func (s *updateServiceGitHubClientStub) DownloadFile(context.Context, string, string, int64) error {
	panic("DownloadFile should not be called when no update is available")
}

func (s *updateServiceGitHubClientStub) FetchChecksumFile(context.Context, string) ([]byte, error) {
	panic("FetchChecksumFile should not be called when no update is available")
}

func TestUpdateServicePerformUpdateNoUpdateReturnsSentinel(t *testing.T) {
	svc := NewUpdateService(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{
			recentReleases: []*GitHubRelease{{
				TagName: "custom-v0.1.164.4",
				Name:    "ModelPort 0.1.164.4",
			}},
		},
		"0.1.164.4",
		"release",
	)

	err := svc.PerformUpdate(context.Background())

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNoUpdateAvailable))
	require.ErrorIs(t, err, ErrNoUpdateAvailable)
}

func newRollbackTestService(current string, releases []*GitHubRelease) *UpdateService {
	return NewUpdateService(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{recentReleases: releases},
		current,
		"release",
	)
}

func TestUpdateServiceListRollbackVersionsFiltersAndCaps(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "custom-v0.1.164.8", PublishedAt: "2026-07-09T00:00:00Z"},
		{TagName: "custom-v0.1.164.7", PublishedAt: "2026-07-08T00:00:00Z"},
		{TagName: "dev-v0.1.164.6-dev.1", PublishedAt: "2026-07-07T12:00:00Z", Prerelease: true},
		{TagName: "custom-v0.1.164.6", PublishedAt: "2026-07-07T00:00:00Z"},
		{TagName: "custom-v0.1.164.5", PublishedAt: "2026-07-06T00:00:00Z", Draft: true},
		{TagName: "custom-v0.1.164.4", PublishedAt: "2026-07-05T00:00:00Z"},
		{TagName: "custom-v0.1.164.4", PublishedAt: "2026-07-05T00:00:00Z"},
		{TagName: "custom-v0.1.164.3", PublishedAt: "2026-07-04T00:00:00Z"},
		{TagName: "custom-v0.1.164.2", PublishedAt: "2026-07-03T00:00:00Z"},
	}
	svc := newRollbackTestService("0.1.164.7", releases)

	versions, err := svc.ListRollbackVersions(context.Background())

	require.NoError(t, err)
	require.Len(t, versions, 3)
	require.Equal(t, "0.1.164.6", versions[0].Version)
	require.Equal(t, "0.1.164.4", versions[1].Version)
	require.Equal(t, "0.1.164.3", versions[2].Version)
}

func TestUpdateServiceListRollbackVersionsSortsUnorderedInput(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "custom-v0.1.164.4"},
		{TagName: "custom-v0.1.164.6"},
		{TagName: "custom-v0.1.164.5"},
	}
	svc := newRollbackTestService("0.1.164.7", releases)

	versions, err := svc.ListRollbackVersions(context.Background())

	require.NoError(t, err)
	require.Len(t, versions, 3)
	require.Equal(t, "0.1.164.6", versions[0].Version)
	require.Equal(t, "0.1.164.5", versions[1].Version)
	require.Equal(t, "0.1.164.4", versions[2].Version)
}

func TestUpdateServiceListRollbackVersionsEmptyWhenNoneOlder(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "custom-v0.1.164.7"},
		{TagName: "custom-v0.1.164.8"},
	}
	svc := newRollbackTestService("0.1.164.7", releases)

	versions, err := svc.ListRollbackVersions(context.Background())

	require.NoError(t, err)
	require.Empty(t, versions)
}

func TestUpdateServiceListRollbackVersionsPropagatesFetchError(t *testing.T) {
	svc := NewUpdateService(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{recentErr: errors.New("github unavailable")},
		"0.1.164.7",
		"release",
	)

	_, err := svc.ListRollbackVersions(context.Background())

	require.Error(t, err)
	require.Contains(t, err.Error(), "github unavailable")
}

func TestUpdateServiceRollbackToVersionRejectsDisallowedTargets(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "custom-v0.1.164.8"},
		{TagName: "custom-v0.1.164.7"},
		{TagName: "custom-v0.1.164.6"},
		{TagName: "custom-v0.1.164.5"},
		{TagName: "custom-v0.1.164.4"},
		{TagName: "custom-v0.1.164.3"},
		{TagName: "custom-v0.1.164.2"},
	}
	svc := newRollbackTestService("0.1.164.7", releases)

	for _, target := range []string{
		"",           // empty
		"0.1.164.7",  // current version
		"v0.1.164.7", // current version with prefix
		"0.1.164.8",  // newer than current
		"0.1.164.2",  // older than the 3 most recent
		"9.9.9",      // nonexistent
	} {
		err := svc.RollbackToVersion(context.Background(), target)
		require.ErrorIs(t, err, ErrRollbackVersionNotAllowed, "target %q should be rejected", target)
	}
}

func TestUpdateServiceRollbackToVersionAcceptsVPrefix(t *testing.T) {
	// No platform asset in the release: the target passes the allowlist check
	// and fails later at asset lookup, proving the version itself was accepted.
	releases := []*GitHubRelease{
		{TagName: "custom-v0.1.164.7"},
		{TagName: "custom-v0.1.164.6"},
	}
	svc := newRollbackTestService("0.1.164.7", releases)

	err := svc.RollbackToVersion(context.Background(), "v0.1.164.6")

	require.Error(t, err)
	require.NotErrorIs(t, err, ErrRollbackVersionNotAllowed)
	require.Contains(t, err.Error(), "online rollback is disabled")
}

func TestUpdateServiceUsesOnlyModelPortDevelopReleases(t *testing.T) {
	client := &updateServiceGitHubClientStub{recentReleases: []*GitHubRelease{
		{TagName: "v0.1.999", Name: "Upstream release"},
		{TagName: "custom-v0.1.164.99", Name: "Stable ModelPort release"},
		{TagName: "dev-v0.1.164.4-dev.14", Name: "ModelPort dev 14", Prerelease: true},
		{TagName: "dev-v0.1.164.4-dev.13", Name: "ModelPort dev 13", Prerelease: true},
	}}
	svc := NewUpdateService(&updateServiceCacheStub{}, client, "0.1.164.4-dev.13", "release")

	info, err := svc.CheckUpdate(context.Background(), true)

	require.NoError(t, err)
	require.Equal(t, modelPortGitHubRepo, client.recentRepo)
	require.Empty(t, client.latestRepo)
	require.True(t, info.HasUpdate)
	require.Equal(t, "0.1.164.4-dev.14", info.LatestVersion)
	require.Equal(t, updateChannelDevelop, info.UpdateChannel)
	require.Equal(t, modelPortReleaseURL("dev-v0.1.164.4-dev.14"), info.ReleaseInfo.HTMLURL)
}

func TestUpdateServiceRejectsOfficialAndWrongChannelTags(t *testing.T) {
	client := &updateServiceGitHubClientStub{recentReleases: []*GitHubRelease{
		{TagName: "v9.9.9"},
		{TagName: "dev-v0.1.164.4-dev.99", Prerelease: true},
	}}
	svc := NewUpdateService(&updateServiceCacheStub{}, client, "0.1.164.4", "release")

	info, err := svc.CheckUpdate(context.Background(), true)

	require.NoError(t, err)
	require.False(t, info.HasUpdate)
	require.Equal(t, "0.1.164.4", info.LatestVersion)
	require.Contains(t, info.Warning, "no ModelPort stable release found")
}

func TestUpdateServiceIgnoresLegacyOfficialCache(t *testing.T) {
	cache := &updateServiceCacheStub{data: `{"latest":"0.1.999","timestamp":9999999999}`}
	client := &updateServiceGitHubClientStub{recentReleases: []*GitHubRelease{
		{TagName: "custom-v0.1.164.5"},
	}}
	svc := NewUpdateService(cache, client, "0.1.164.4", "release")

	info, err := svc.CheckUpdate(context.Background(), false)

	require.NoError(t, err)
	require.Equal(t, modelPortGitHubRepo, client.recentRepo)
	require.Equal(t, "0.1.164.5", info.LatestVersion)
	require.False(t, info.Cached)
}

func TestCompareModelPortDevelopmentVersions(t *testing.T) {
	require.Equal(t, -1, compareVersions("0.1.164.4-dev.9", "0.1.164.4-dev.10"))
	require.Equal(t, 1, compareVersions("0.1.164.5-dev.1", "0.1.164.4-dev.99"))
	require.Equal(t, 0, compareVersions("0.1.164.4-dev.10", "0.1.164.4-dev.10"))
}

func TestUpdateServiceQueuesPinnedDockerImageVersion(t *testing.T) {
	requestFile := filepath.Join(t.TempDir(), "update-request")
	t.Setenv(modelPortUpdateModeEnv, updateModeDocker)
	t.Setenv(modelPortRequestFileEnv, requestFile)
	client := &updateServiceGitHubClientStub{recentReleases: []*GitHubRelease{
		{TagName: "dev-v0.1.164.4-dev.14", Prerelease: true},
	}}
	svc := NewUpdateService(&updateServiceCacheStub{}, client, "0.1.164.4-dev.13", "release")

	require.NoError(t, svc.PerformUpdate(context.Background()))
	request, err := os.ReadFile(requestFile)
	require.NoError(t, err)
	require.Equal(t, "0.1.164.4-dev.14\n", string(request))
}
