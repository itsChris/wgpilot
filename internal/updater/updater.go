package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const (
	defaultRepo    = "itsChris/wgpilot"
	githubAPIBase  = "https://api.github.com"
	downloadBase   = "https://github.com"
	requestTimeout = 30 * time.Second
)

// ReleaseInfo holds metadata about a GitHub release.
type ReleaseInfo struct {
	TagName     string `json:"tag_name"`
	PublishedAt string `json:"published_at"`
	HTMLURL     string `json:"html_url"`
}

// UpdateResult holds the outcome of an update check.
type UpdateResult struct {
	CurrentVersion string
	LatestVersion  string
	UpdateAvailable bool
	ReleaseURL     string
}

// Updater checks for and applies self-updates from GitHub releases.
type Updater struct {
	repo       string
	httpClient *http.Client
	logger     *slog.Logger
}

// NewUpdater creates a new Updater for the given GitHub repository.
func NewUpdater(logger *slog.Logger) (*Updater, error) {
	return &Updater{
		repo: defaultRepo,
		httpClient: &http.Client{
			Timeout: requestTimeout,
		},
		logger: logger,
	}, nil
}

// CheckLatest queries the GitHub releases API and compares the latest
// release tag with the current version.
func (u *Updater) CheckLatest(ctx context.Context, currentVersion string) (*UpdateResult, error) {
	release, err := u.fetchLatestRelease(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch latest release: %w", err)
	}

	latest := normalizeVersion(release.TagName)
	current := normalizeVersion(currentVersion)

	return &UpdateResult{
		CurrentVersion:  current,
		LatestVersion:   latest,
		UpdateAvailable: latest != current && current != "dev",
		ReleaseURL:      release.HTMLURL,
	}, nil
}

// Update downloads the latest binary, verifies it, replaces the current
// binary, and signals systemd to restart.
func (u *Updater) Update(ctx context.Context, currentVersion string) (*UpdateResult, error) {
	result, err := u.CheckLatest(ctx, currentVersion)
	if err != nil {
		return nil, err
	}

	if !result.UpdateAvailable {
		return result, nil
	}

	u.logger.Info("update_downloading",
		"current", result.CurrentVersion,
		"latest", result.LatestVersion,
		"component", "updater",
	)

	binaryURL := u.binaryURL(result.LatestVersion)
	tmpFile, err := u.downloadBinary(ctx, binaryURL)
	if err != nil {
		return nil, fmt.Errorf("download binary: %w", err)
	}
	defer os.Remove(tmpFile)

	if err := u.verifyBinary(tmpFile); err != nil {
		return nil, fmt.Errorf("verify binary: %w", err)
	}

	selfPath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("get executable path: %w", err)
	}

	if err := replaceBinary(tmpFile, selfPath); err != nil {
		return nil, fmt.Errorf("replace binary: %w", err)
	}

	u.logger.Info("update_complete",
		"version", result.LatestVersion,
		"component", "updater",
	)

	return result, nil
}

func (u *Updater) fetchLatestRelease(ctx context.Context) (*ReleaseInfo, error) {
	url := fmt.Sprintf("%s/repos/%s/releases/latest", githubAPIBase, u.repo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "wgpilot-updater")

	resp, err := u.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github api request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("github api returned %d: %s", resp.StatusCode, string(body))
	}

	var release ReleaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("decode release json: %w", err)
	}

	if release.TagName == "" {
		return nil, fmt.Errorf("release tag_name is empty")
	}

	return &release, nil
}

func (u *Updater) downloadBinary(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("create download request: %w", err)
	}
	req.Header.Set("User-Agent", "wgpilot-updater")

	resp, err := u.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("download request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	tmpFile, err := os.CreateTemp("", "wgpilot-update-*")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	defer tmpFile.Close()

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("write temp file: %w", err)
	}

	if err := os.Chmod(tmpFile.Name(), 0755); err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("chmod temp file: %w", err)
	}

	return tmpFile.Name(), nil
}

func (u *Updater) verifyBinary(path string) error {
	cmd := exec.Command(path, "version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("binary verification failed: %w (output: %s)", err, string(out))
	}
	u.logger.Debug("update_binary_verified",
		"output", strings.TrimSpace(string(out)),
		"component", "updater",
	)
	return nil
}

func (u *Updater) binaryURL(version string) string {
	archSuffix := archBinarySuffix()
	tag := "v" + version
	if strings.HasPrefix(version, "v") {
		tag = version
	}
	return fmt.Sprintf("%s/%s/releases/download/%s/wgpilot_%s",
		downloadBase, u.repo, tag, archSuffix)
}

func replaceBinary(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer srcFile.Close()

	// Write to a temporary file next to the destination, then rename
	// for atomic replacement.
	tmpDst := dst + ".new"
	dstFile, err := os.OpenFile(tmpDst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("create temp destination: %w", err)
	}

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		dstFile.Close()
		os.Remove(tmpDst)
		return fmt.Errorf("copy binary: %w", err)
	}
	dstFile.Close()

	if err := os.Rename(tmpDst, dst); err != nil {
		os.Remove(tmpDst)
		return fmt.Errorf("rename binary: %w", err)
	}

	return nil
}

// normalizeVersion strips a leading "v" from a version string.
func normalizeVersion(v string) string {
	return strings.TrimPrefix(v, "v")
}

// archBinarySuffix returns the binary suffix for the current OS/arch.
func archBinarySuffix() string {
	goarch := runtime.GOARCH
	switch goarch {
	case "amd64":
		return "linux_amd64"
	case "arm64":
		return "linux_arm64"
	case "arm":
		return "linux_arm7"
	default:
		return "linux_" + goarch
	}
}
