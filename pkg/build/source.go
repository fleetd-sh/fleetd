package build

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
)

type SourceType string

const (
	SourceTypeUnspecified SourceType = ""
	SourceTypeGit         SourceType = "git"
	SourceTypeTarball     SourceType = "tarball"
	SourceTypeBinary      SourceType = "binary"
	SourceTypeOCI         SourceType = "oci"
)

type Source struct {
	Type      SourceType
	URL       string
	Reference string
	Signature []byte
}

type SourceValidator interface {
	Validate(spec *Source) error
}

type DefaultSourceValidator struct {
	allowedDomains []string
	maxSize        int64
}

func (v *DefaultSourceValidator) Validate(spec *Source) error {
	if spec == nil {
		return fmt.Errorf("source spec is nil")
	}

	if spec.URL == "" {
		return fmt.Errorf("source URL is required")
	}

	// Validate URL
	u, err := url.Parse(spec.URL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	// Check allowed domains
	if len(v.allowedDomains) > 0 {
		allowed := false
		for _, domain := range v.allowedDomains {
			if strings.HasSuffix(u.Host, domain) {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("domain not allowed: %s", u.Host)
		}
	}

	return nil
}

type ProgressWriter struct {
	Total      int64
	Current    int64
	OnProgress func(current, total int64)
}

func (pw *ProgressWriter) Write(p []byte) (int, error) {
	n := len(p)
	pw.Current += int64(n)
	if pw.OnProgress != nil {
		pw.OnProgress(pw.Current, pw.Total)
	}
	return n, nil
}

func fetchWithRetry(ctx context.Context, url string) (io.ReadCloser, error) {
	var body io.ReadCloser

	operation := func() error {
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("request failed: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return fmt.Errorf("unexpected status: %s", resp.Status)
		}

		body = resp.Body
		return nil
	}

	b := backoff.NewExponentialBackOff()
	b.MaxElapsedTime = 5 * time.Minute

	if err := backoff.Retry(operation, b); err != nil {
		return nil, err
	}

	return body, nil
}

// fetchSource downloads or clones the source code based on the source spec
func fetchSource(ctx context.Context, destDir string, spec *Source) error {
	switch spec.Type {
	case SourceTypeGit:
		return fetchGitSource(ctx, destDir, spec)
	case SourceTypeTarball:
		return fetchTarballSource(ctx, destDir, spec)
	case SourceTypeBinary:
		return fetchBinarySource(destDir, spec)
	default:
		return fmt.Errorf("unsupported source type: %v", spec.Type)
	}
}

func fetchGitSource(ctx context.Context, destDir string, spec *Source) error {
	cmd := exec.CommandContext(ctx, "git", "clone", spec.URL, destDir)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git clone failed: %w", err)
	}

	if spec.Reference != "" {
		cmd = exec.CommandContext(ctx, "git", "checkout", spec.Reference)
		cmd.Dir = destDir
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("git checkout failed: %w", err)
		}
	}

	return nil
}

func fetchTarballSource(ctx context.Context, destDir string, spec *Source) error {
	resp, err := http.Get(spec.URL)
	if err != nil {
		return fmt.Errorf("failed to download tarball: %w", err)
	}
	defer resp.Body.Close()

	tarPath := filepath.Join(destDir, "source.tar.gz")
	f, err := os.Create(tarPath)
	if err != nil {
		return fmt.Errorf("failed to create tarball file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("failed to write tarball: %w", err)
	}

	cmd := exec.CommandContext(ctx, "tar", "xzf", tarPath, "-C", destDir)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to extract tarball: %w", err)
	}

	return nil
}

func fetchBinarySource(destDir string, spec *Source) error {
	resp, err := http.Get(spec.URL)
	if err != nil {
		return fmt.Errorf("failed to download binary: %w", err)
	}
	defer resp.Body.Close()

	binPath := filepath.Join(destDir, "binary")
	f, err := os.Create(binPath)
	if err != nil {
		return fmt.Errorf("failed to create binary file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("failed to write binary: %w", err)
	}

	return nil
}
