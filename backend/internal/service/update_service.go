package service

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var (
	ErrNoUpdateAvailable         = infraerrors.Conflict("ALREADY_UP_TO_DATE", "no update available; current version is latest")
	ErrRollbackVersionNotAllowed = infraerrors.BadRequest("ROLLBACK_VERSION_NOT_ALLOWED", "version is not in the allowed rollback list")
)

const (
	updateCacheTTL          = 1200 // 20 minutes
	modelPortGitHubRepo     = "abingooo/modelport"
	modelPortUpdateModeEnv  = "MODELPORT_UPDATE_MODE"
	modelPortRequestFileEnv = "MODELPORT_UPDATE_REQUEST_FILE"
	updateModeManual        = "manual"
	updateModeBinary        = "binary"
	updateModeDocker        = "docker"
	updateChannelStable     = "stable"
	updateChannelDevelop    = "develop"

	// Security: allowed download domains for updates
	allowedDownloadHost = "github.com"
	allowedAssetHost    = "objects.githubusercontent.com"

	// Security: max download size (500MB)
	maxDownloadSize = 500 * 1024 * 1024

	// Rollback: expose at most the 3 most recent versions older than current
	maxRollbackVersions = 3
	// Fetch a few extra releases so filtering (current/newer/prerelease) still leaves enough candidates
	rollbackFetchPageSize = 50
)

var (
	modelPortStableVersionPattern  = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)\.(\d+)$`)
	modelPortDevelopVersionPattern = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)\.(\d+)-dev\.(\d+)$`)
)

// UpdateCache defines cache operations for update service
type UpdateCache interface {
	GetUpdateInfo(ctx context.Context) (string, error)
	SetUpdateInfo(ctx context.Context, data string, ttl time.Duration) error
}

// GitHubReleaseClient 获取 GitHub release 信息的接口
type GitHubReleaseClient interface {
	FetchLatestRelease(ctx context.Context, repo string) (*GitHubRelease, error)
	FetchRecentReleases(ctx context.Context, repo string, perPage int) ([]*GitHubRelease, error)
	DownloadFile(ctx context.Context, url, dest string, maxSize int64) error
	FetchChecksumFile(ctx context.Context, url string) ([]byte, error)
}

// UpdateService handles software updates
type UpdateService struct {
	cache          UpdateCache
	githubClient   GitHubReleaseClient
	currentVersion string
	buildType      string // "source" for manual builds, "release" for CI builds
	updateChannel  string
	updateMode     string
	updateRequest  string
}

// NewUpdateService creates a new UpdateService
func NewUpdateService(cache UpdateCache, githubClient GitHubReleaseClient, version, buildType string) *UpdateService {
	updateMode := strings.ToLower(strings.TrimSpace(os.Getenv(modelPortUpdateModeEnv)))
	requestFile := strings.TrimSpace(os.Getenv(modelPortRequestFileEnv))
	if updateMode != updateModeBinary && updateMode != updateModeDocker {
		updateMode = updateModeManual
	}
	if updateMode == updateModeDocker && (!filepath.IsAbs(requestFile) || requestFile == string(filepath.Separator)) {
		updateMode = updateModeManual
		requestFile = ""
	}

	return &UpdateService{
		cache:          cache,
		githubClient:   githubClient,
		currentVersion: version,
		buildType:      buildType,
		updateChannel:  modelPortUpdateChannel(version),
		updateMode:     updateMode,
		updateRequest:  requestFile,
	}
}

// UpdateInfo contains update information
type UpdateInfo struct {
	CurrentVersion string       `json:"current_version"`
	LatestVersion  string       `json:"latest_version"`
	HasUpdate      bool         `json:"has_update"`
	ReleaseInfo    *ReleaseInfo `json:"release_info,omitempty"`
	Cached         bool         `json:"cached"`
	Warning        string       `json:"warning,omitempty"`
	BuildType      string       `json:"build_type"` // "source" or "release"
	UpdateMode     string       `json:"update_mode"`
	UpdateChannel  string       `json:"update_channel,omitempty"`
	Repository     string       `json:"repository"`
}

// ReleaseInfo contains GitHub release details
type ReleaseInfo struct {
	Name        string  `json:"name"`
	Body        string  `json:"body"`
	PublishedAt string  `json:"published_at"`
	HTMLURL     string  `json:"html_url"`
	Assets      []Asset `json:"assets,omitempty"`
}

// Asset represents a release asset
type Asset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"download_url"`
	Size        int64  `json:"size"`
}

// GitHubRelease represents GitHub API response
type GitHubRelease struct {
	TagName     string        `json:"tag_name"`
	Name        string        `json:"name"`
	Body        string        `json:"body"`
	PublishedAt string        `json:"published_at"`
	HTMLURL     string        `json:"html_url"`
	Draft       bool          `json:"draft"`
	Prerelease  bool          `json:"prerelease"`
	Assets      []GitHubAsset `json:"assets"`
}

// RollbackVersion describes a release version the system can roll back to
type RollbackVersion struct {
	Version     string `json:"version"` // without "v" prefix, e.g. "0.1.146"
	PublishedAt string `json:"published_at"`
	HTMLURL     string `json:"html_url"`
}

type GitHubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// CheckUpdate checks for available updates
func (s *UpdateService) CheckUpdate(ctx context.Context, force bool) (*UpdateInfo, error) {
	if s.updateChannel == "" {
		return s.currentUpdateInfo("This build does not use a ModelPort release version"), nil
	}

	// Try cache first
	if !force {
		if cached, err := s.getFromCache(ctx); err == nil && cached != nil {
			return cached, nil
		}
	}

	// Fetch from GitHub
	info, err := s.fetchLatestRelease(ctx)
	if err != nil {
		// Return cached on error
		if cached, cacheErr := s.getFromCache(ctx); cacheErr == nil && cached != nil {
			cached.Warning = "Using cached data: " + err.Error()
			return cached, nil
		}
		return &UpdateInfo{
			CurrentVersion: s.currentVersion,
			LatestVersion:  s.currentVersion,
			HasUpdate:      false,
			Warning:        err.Error(),
			BuildType:      s.buildType,
			UpdateMode:     s.updateMode,
			UpdateChannel:  s.updateChannel,
			Repository:     modelPortGitHubRepo,
		}, nil
	}

	// Cache result
	s.saveToCache(ctx, info)
	return info, nil
}

// PerformUpdate downloads and applies the update
// Uses atomic file replacement pattern for safe in-place updates
func (s *UpdateService) PerformUpdate(ctx context.Context) error {
	info, err := s.CheckUpdate(ctx, true)
	if err != nil {
		return err
	}

	if !info.HasUpdate {
		return ErrNoUpdateAvailable
	}

	switch s.updateMode {
	case updateModeDocker:
		return s.queueDockerUpdate(info.LatestVersion)
	case updateModeBinary:
		return s.applyReleaseAssets(ctx, info.ReleaseInfo.Assets)
	default:
		return fmt.Errorf("online update is disabled for this deployment")
	}
}

func (s *UpdateService) UpdateMode() string {
	return s.updateMode
}

// applyReleaseAssets downloads the platform archive from the given release assets,
// verifies its checksum, and atomically swaps the running binary.
// Shared by PerformUpdate (latest) and RollbackToVersion (specific older version).
func (s *UpdateService) applyReleaseAssets(ctx context.Context, releaseAssets []Asset) error {
	// Find matching archive and checksum for current platform
	archiveName := s.getArchiveName()
	var downloadURL string
	var checksumURL string

	for _, asset := range releaseAssets {
		if strings.Contains(asset.Name, archiveName) && !strings.HasSuffix(asset.Name, ".txt") {
			downloadURL = asset.DownloadURL
		}
		if asset.Name == "checksums.txt" {
			checksumURL = asset.DownloadURL
		}
	}

	if downloadURL == "" {
		return fmt.Errorf("no compatible release found for %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	// SECURITY: Validate download URL is from trusted domain
	if err := validateDownloadURL(downloadURL); err != nil {
		return fmt.Errorf("invalid download URL: %w", err)
	}
	if checksumURL != "" {
		if err := validateDownloadURL(checksumURL); err != nil {
			return fmt.Errorf("invalid checksum URL: %w", err)
		}
	}

	// Get current executable path
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return fmt.Errorf("failed to resolve symlinks: %w", err)
	}

	exeDir := filepath.Dir(exePath)

	// Create temp directory in the SAME directory as executable
	// This ensures os.Rename is atomic (same filesystem)
	tempDir, err := os.MkdirTemp(exeDir, ".sub2api-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	// Download archive
	archivePath := filepath.Join(tempDir, filepath.Base(downloadURL))
	if err := s.downloadFile(ctx, downloadURL, archivePath); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	// Verify checksum if available
	if checksumURL != "" {
		if err := s.verifyChecksum(ctx, archivePath, checksumURL); err != nil {
			return fmt.Errorf("checksum verification failed: %w", err)
		}
	}

	// Extract binary from archive
	newBinaryPath := filepath.Join(tempDir, "sub2api")
	if err := s.extractBinary(archivePath, newBinaryPath); err != nil {
		return fmt.Errorf("extraction failed: %w", err)
	}

	// Set executable permission before replacement
	if err := os.Chmod(newBinaryPath, 0755); err != nil {
		return fmt.Errorf("chmod failed: %w", err)
	}

	// Atomic replacement using rename pattern:
	// 1. Rename current -> backup (atomic on Unix)
	// 2. Rename new -> current (atomic on Unix, same filesystem)
	// If step 2 fails, restore backup
	backupPath := exePath + ".backup"

	// Remove old backup if exists
	_ = os.Remove(backupPath)

	// Step 1: Move current binary to backup
	if err := os.Rename(exePath, backupPath); err != nil {
		return fmt.Errorf("backup failed: %w", err)
	}

	// Step 2: Move new binary to target location (atomic, same filesystem)
	if err := os.Rename(newBinaryPath, exePath); err != nil {
		// Restore backup on failure
		if restoreErr := os.Rename(backupPath, exePath); restoreErr != nil {
			return fmt.Errorf("replace failed and restore failed: %w (restore error: %v)", err, restoreErr)
		}
		return fmt.Errorf("replace failed (restored backup): %w", err)
	}

	// Success - backup file is kept for rollback capability
	// It will be cleaned up on next successful update
	return nil
}

// Rollback restores the previous version
func (s *UpdateService) Rollback() error {
	if s.updateMode != updateModeBinary {
		return fmt.Errorf("local binary rollback is disabled for this deployment")
	}
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return fmt.Errorf("failed to resolve symlinks: %w", err)
	}

	backupFile := exePath + ".backup"
	if _, err := os.Stat(backupFile); os.IsNotExist(err) {
		return fmt.Errorf("no backup found")
	}

	// Replace current with backup
	if err := os.Rename(backupFile, exePath); err != nil {
		return fmt.Errorf("rollback failed: %w", err)
	}

	return nil
}

// ListRollbackVersions returns up to maxRollbackVersions release versions that are
// strictly older than the current version (the current version itself is excluded),
// newest first. Releases outside the current ModelPort channel are skipped.
func (s *UpdateService) ListRollbackVersions(ctx context.Context) ([]RollbackVersion, error) {
	if s.updateChannel == "" {
		return []RollbackVersion{}, nil
	}
	releases, err := s.fetchRollbackCandidates(ctx)
	if err != nil {
		return nil, err
	}

	versions := make([]RollbackVersion, 0, len(releases))
	for _, r := range releases {
		versions = append(versions, RollbackVersion{
			Version:     modelPortReleaseVersion(r, s.updateChannel),
			PublishedAt: r.PublishedAt,
			HTMLURL:     modelPortReleaseURL(r.TagName),
		})
	}
	return versions, nil
}

// RollbackToVersion downloads and installs a specific older version.
// The target must be one of the versions returned by ListRollbackVersions;
// anything else (including the current version) is rejected.
func (s *UpdateService) RollbackToVersion(ctx context.Context, version string) error {
	target := strings.TrimPrefix(strings.TrimSpace(version), "v")
	if target == "" {
		return ErrRollbackVersionNotAllowed
	}

	releases, err := s.fetchRollbackCandidates(ctx)
	if err != nil {
		return err
	}

	var match *GitHubRelease
	for _, r := range releases {
		if modelPortReleaseVersion(r, s.updateChannel) == target {
			match = r
			break
		}
	}
	if match == nil {
		return ErrRollbackVersionNotAllowed
	}

	assets := make([]Asset, len(match.Assets))
	for i, a := range match.Assets {
		assets[i] = Asset{
			Name:        a.Name,
			DownloadURL: a.BrowserDownloadURL,
			Size:        a.Size,
		}
	}

	if s.updateMode == updateModeDocker {
		return s.queueDockerUpdate(target)
	}
	if s.updateMode == updateModeBinary {
		return s.applyReleaseAssets(ctx, assets)
	}
	return fmt.Errorf("online rollback is disabled for this deployment")
}

// fetchRollbackCandidates fetches recent releases and keeps the newest
// maxRollbackVersions entries strictly older than the current version.
func (s *UpdateService) fetchRollbackCandidates(ctx context.Context) ([]*GitHubRelease, error) {
	releases, err := s.githubClient.FetchRecentReleases(ctx, modelPortGitHubRepo, rollbackFetchPageSize)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool, len(releases))
	candidates := make([]*GitHubRelease, 0, maxRollbackVersions)
	for _, r := range releases {
		if r == nil || r.Draft {
			continue
		}
		v := modelPortReleaseVersion(r, s.updateChannel)
		if v == "" || seen[v] {
			continue
		}
		// Only versions strictly older than current (also excludes current itself)
		if compareVersions(v, s.currentVersion) >= 0 {
			continue
		}
		seen[v] = true
		candidates = append(candidates, r)
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		return compareVersions(
			modelPortReleaseVersion(candidates[i], s.updateChannel),
			modelPortReleaseVersion(candidates[j], s.updateChannel),
		) > 0
	})

	if len(candidates) > maxRollbackVersions {
		candidates = candidates[:maxRollbackVersions]
	}
	return candidates, nil
}

func (s *UpdateService) fetchLatestRelease(ctx context.Context) (*UpdateInfo, error) {
	releases, err := s.githubClient.FetchRecentReleases(ctx, modelPortGitHubRepo, rollbackFetchPageSize)
	if err != nil {
		return nil, err
	}
	var release *GitHubRelease
	latestVersion := ""
	for _, candidate := range releases {
		if candidate == nil || candidate.Draft {
			continue
		}
		version := modelPortReleaseVersion(candidate, s.updateChannel)
		if version != "" && (release == nil || compareVersions(version, latestVersion) > 0) {
			release = candidate
			latestVersion = version
		}
	}
	if release == nil {
		return nil, fmt.Errorf("no ModelPort %s release found", s.updateChannel)
	}

	releaseURL := modelPortReleaseURL(release.TagName)

	assets := make([]Asset, len(release.Assets))
	for i, a := range release.Assets {
		assets[i] = Asset{
			Name:        a.Name,
			DownloadURL: a.BrowserDownloadURL,
			Size:        a.Size,
		}
	}

	return &UpdateInfo{
		CurrentVersion: s.currentVersion,
		LatestVersion:  latestVersion,
		HasUpdate:      compareVersions(s.currentVersion, latestVersion) < 0,
		ReleaseInfo: &ReleaseInfo{
			Name:        release.Name,
			Body:        release.Body,
			PublishedAt: release.PublishedAt,
			HTMLURL:     releaseURL,
			Assets:      assets,
		},
		Cached:        false,
		BuildType:     s.buildType,
		UpdateMode:    s.updateMode,
		UpdateChannel: s.updateChannel,
		Repository:    modelPortGitHubRepo,
	}, nil
}

func (s *UpdateService) downloadFile(ctx context.Context, downloadURL, dest string) error {
	return s.githubClient.DownloadFile(ctx, downloadURL, dest, maxDownloadSize)
}

func (s *UpdateService) getArchiveName() string {
	osName := runtime.GOOS
	arch := runtime.GOARCH
	return fmt.Sprintf("%s_%s", osName, arch)
}

// validateDownloadURL checks if the URL is from an allowed domain
// SECURITY: This prevents SSRF and ensures downloads only come from trusted GitHub domains
func validateDownloadURL(rawURL string) error {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	// Must be HTTPS
	if parsedURL.Scheme != "https" {
		return fmt.Errorf("only HTTPS URLs are allowed")
	}

	// Check against allowed hosts
	host := parsedURL.Host
	// GitHub release URLs can be from github.com or objects.githubusercontent.com
	if host != allowedDownloadHost &&
		!strings.HasSuffix(host, "."+allowedDownloadHost) &&
		host != allowedAssetHost &&
		!strings.HasSuffix(host, "."+allowedAssetHost) {
		return fmt.Errorf("download from untrusted host: %s", host)
	}

	return nil
}

func (s *UpdateService) verifyChecksum(ctx context.Context, filePath, checksumURL string) error {
	// Download checksums file
	checksumData, err := s.githubClient.FetchChecksumFile(ctx, checksumURL)
	if err != nil {
		return fmt.Errorf("failed to download checksums: %w", err)
	}

	// Calculate file hash
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	actualHash := hex.EncodeToString(h.Sum(nil))

	// Find expected hash in checksums file
	fileName := filepath.Base(filePath)
	scanner := bufio.NewScanner(strings.NewReader(string(checksumData)))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Fields(line)
		if len(parts) == 2 && parts[1] == fileName {
			if parts[0] == actualHash {
				return nil
			}
			return fmt.Errorf("checksum mismatch: expected %s, got %s", parts[0], actualHash)
		}
	}

	return fmt.Errorf("checksum not found for %s", fileName)
}

func (s *UpdateService) extractBinary(archivePath, destPath string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	var reader io.Reader = f

	// Handle gzip compression
	if strings.HasSuffix(archivePath, ".gz") || strings.HasSuffix(archivePath, ".tar.gz") || strings.HasSuffix(archivePath, ".tgz") {
		gzr, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		defer func() { _ = gzr.Close() }()
		reader = gzr
	}

	// Handle tar archive
	if strings.Contains(archivePath, ".tar") {
		tr := tar.NewReader(reader)
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}

			// SECURITY: Prevent Zip Slip / Path Traversal attack
			// Only allow files with safe base names, no directory traversal
			baseName := filepath.Base(hdr.Name)

			// Check for path traversal attempts
			if strings.Contains(hdr.Name, "..") {
				return fmt.Errorf("path traversal attempt detected: %s", hdr.Name)
			}

			// Validate the entry is a regular file
			if hdr.Typeflag != tar.TypeReg {
				continue // Skip directories and special files
			}

			// Only extract the specific binary we need
			if baseName == "sub2api" || baseName == "sub2api.exe" {
				// Additional security: limit file size (max 500MB)
				const maxBinarySize = 500 * 1024 * 1024
				if hdr.Size > maxBinarySize {
					return fmt.Errorf("binary too large: %d bytes (max %d)", hdr.Size, maxBinarySize)
				}

				out, err := os.Create(destPath)
				if err != nil {
					return err
				}

				// Use LimitReader to prevent decompression bombs
				limited := io.LimitReader(tr, maxBinarySize)
				if _, err := io.Copy(out, limited); err != nil {
					_ = out.Close()
					return err
				}
				if err := out.Close(); err != nil {
					return err
				}
				return nil
			}
		}
		return fmt.Errorf("binary not found in archive")
	}

	// Direct copy for non-tar files (with size limit)
	const maxBinarySize = 500 * 1024 * 1024
	out, err := os.Create(destPath)
	if err != nil {
		return err
	}

	limited := io.LimitReader(reader, maxBinarySize)
	if _, err := io.Copy(out, limited); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func (s *UpdateService) getFromCache(ctx context.Context) (*UpdateInfo, error) {
	data, err := s.cache.GetUpdateInfo(ctx)
	if err != nil {
		return nil, err
	}

	var cached struct {
		Latest      string       `json:"latest"`
		ReleaseInfo *ReleaseInfo `json:"release_info"`
		Timestamp   int64        `json:"timestamp"`
		Repository  string       `json:"repository"`
		Channel     string       `json:"channel"`
	}
	if err := json.Unmarshal([]byte(data), &cached); err != nil {
		return nil, err
	}

	if time.Now().Unix()-cached.Timestamp > updateCacheTTL {
		return nil, fmt.Errorf("cache expired")
	}
	if cached.Repository != modelPortGitHubRepo || cached.Channel != s.updateChannel {
		return nil, fmt.Errorf("cache belongs to a different update source")
	}
	if !modelPortVersionMatchesChannel(cached.Latest, s.updateChannel) {
		return nil, fmt.Errorf("cache contains an invalid ModelPort version")
	}
	if cached.ReleaseInfo != nil {
		cached.ReleaseInfo.HTMLURL = modelPortReleaseURL(modelPortReleaseTag(cached.Latest, s.updateChannel))
	}

	return &UpdateInfo{
		CurrentVersion: s.currentVersion,
		LatestVersion:  cached.Latest,
		HasUpdate:      compareVersions(s.currentVersion, cached.Latest) < 0,
		ReleaseInfo:    cached.ReleaseInfo,
		Cached:         true,
		BuildType:      s.buildType,
		UpdateMode:     s.updateMode,
		UpdateChannel:  s.updateChannel,
		Repository:     modelPortGitHubRepo,
	}, nil
}

func (s *UpdateService) saveToCache(ctx context.Context, info *UpdateInfo) {
	cacheData := struct {
		Latest      string       `json:"latest"`
		ReleaseInfo *ReleaseInfo `json:"release_info"`
		Timestamp   int64        `json:"timestamp"`
		Repository  string       `json:"repository"`
		Channel     string       `json:"channel"`
	}{
		Latest:      info.LatestVersion,
		ReleaseInfo: info.ReleaseInfo,
		Timestamp:   time.Now().Unix(),
		Repository:  modelPortGitHubRepo,
		Channel:     s.updateChannel,
	}

	data, _ := json.Marshal(cacheData)
	_ = s.cache.SetUpdateInfo(ctx, string(data), time.Duration(updateCacheTTL)*time.Second)
}

type modelPortVersion struct {
	parts       [4]int
	development bool
	devNumber   int
}

func modelPortUpdateChannel(version string) string {
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	if modelPortDevelopVersionPattern.MatchString(version) {
		return updateChannelDevelop
	}
	if modelPortStableVersionPattern.MatchString(version) {
		return updateChannelStable
	}
	return ""
}

func modelPortVersionMatchesChannel(version, channel string) bool {
	return modelPortUpdateChannel(version) == channel
}

func modelPortReleaseVersion(release *GitHubRelease, channel string) string {
	if release == nil {
		return ""
	}

	var version string
	switch channel {
	case updateChannelDevelop:
		if !release.Prerelease || !strings.HasPrefix(release.TagName, "dev-v") {
			return ""
		}
		version = strings.TrimPrefix(release.TagName, "dev-v")
	case updateChannelStable:
		if release.Prerelease || !strings.HasPrefix(release.TagName, "custom-v") {
			return ""
		}
		version = strings.TrimPrefix(release.TagName, "custom-v")
	default:
		return ""
	}
	if !modelPortVersionMatchesChannel(version, channel) {
		return ""
	}
	return version
}

func modelPortReleaseURL(tag string) string {
	return fmt.Sprintf("https://github.com/%s/releases/tag/%s", modelPortGitHubRepo, url.PathEscape(tag))
}

func modelPortReleaseTag(version, channel string) string {
	if channel == updateChannelDevelop {
		return "dev-v" + version
	}
	return "custom-v" + version
}

func parseModelPortVersion(version string) (modelPortVersion, bool) {
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	matches := modelPortStableVersionPattern.FindStringSubmatch(version)
	development := false
	if matches == nil {
		matches = modelPortDevelopVersionPattern.FindStringSubmatch(version)
		development = true
	}
	if matches == nil {
		return modelPortVersion{}, false
	}

	parsed := modelPortVersion{development: development}
	for index := range parsed.parts {
		value, err := strconv.Atoi(matches[index+1])
		if err != nil {
			return modelPortVersion{}, false
		}
		parsed.parts[index] = value
	}
	if development {
		value, err := strconv.Atoi(matches[5])
		if err != nil {
			return modelPortVersion{}, false
		}
		parsed.devNumber = value
	}
	return parsed, true
}

func compareVersions(current, latest string) int {
	currentVersion, currentOK := parseModelPortVersion(current)
	latestVersion, latestOK := parseModelPortVersion(latest)
	if !currentOK || !latestOK {
		return 0
	}

	for index := range currentVersion.parts {
		if currentVersion.parts[index] < latestVersion.parts[index] {
			return -1
		}
		if currentVersion.parts[index] > latestVersion.parts[index] {
			return 1
		}
	}
	if currentVersion.development != latestVersion.development {
		if currentVersion.development {
			return -1
		}
		return 1
	}
	if currentVersion.devNumber < latestVersion.devNumber {
		return -1
	}
	if currentVersion.devNumber > latestVersion.devNumber {
		return 1
	}
	return 0
}

func (s *UpdateService) currentUpdateInfo(warning string) *UpdateInfo {
	return &UpdateInfo{
		CurrentVersion: s.currentVersion,
		LatestVersion:  s.currentVersion,
		HasUpdate:      false,
		Warning:        warning,
		BuildType:      s.buildType,
		UpdateMode:     s.updateMode,
		UpdateChannel:  s.updateChannel,
		Repository:     modelPortGitHubRepo,
	}
}

func (s *UpdateService) queueDockerUpdate(version string) error {
	if !modelPortVersionMatchesChannel(version, s.updateChannel) {
		return fmt.Errorf("version is not valid for the %s update channel", s.updateChannel)
	}
	if s.updateRequest == "" {
		return fmt.Errorf("docker update request file is not configured")
	}

	directory := filepath.Dir(s.updateRequest)
	if err := os.MkdirAll(directory, 0750); err != nil {
		return fmt.Errorf("create update request directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".modelport-update-*")
	if err != nil {
		return fmt.Errorf("create update request: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()

	if err := temporary.Chmod(0600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("secure update request: %w", err)
	}
	if _, err := io.WriteString(temporary, version+"\n"); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write update request: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync update request: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close update request: %w", err)
	}
	if err := os.Rename(temporaryPath, s.updateRequest); err != nil {
		return fmt.Errorf("publish update request: %w", err)
	}
	return nil
}
