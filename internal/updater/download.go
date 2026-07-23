package updater

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// download retrieves the URL and writes the response body to dstPath.
// It enforces the configured MaxDownloadBytes limit. It retries on
// transient errors up to DefaultMaxRetries times.
func (updater *Updater) download(ctx context.Context, url, dstPath string) error {
	var lastErr error
	for attempt := range DefaultMaxRetries {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if attempt > 0 {
			updater.logger.Warn("retrying download", "url", url, "attempt", attempt+1)
			time.Sleep(DefaultRetryDelay)
		}

		request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}

		response, err := updater.client.Do(request)
		if err != nil {
			lastErr = fmt.Errorf("http get: %w", err)
			continue
		}

		if response.StatusCode != http.StatusOK {
			response.Body.Close()
			lastErr = fmt.Errorf("http status %d", response.StatusCode)
			continue
		}

		out, err := os.Create(dstPath)
		if err != nil {
			response.Body.Close()
			return fmt.Errorf("create file: %w", err)
		}

		limit := updater.config.MaxDownloadBytes
		// Read limit+1 bytes so we can detect ≥limit+1 (oversize) vs ≤limit (ok).
		written, err := io.Copy(out, io.LimitReader(response.Body, limit+1))
		response.Body.Close()
		out.Close()
		if err != nil {
			lastErr = fmt.Errorf("write body: %w", err)
			os.Remove(dstPath)
			continue
		}
		if written > limit {
			os.Remove(dstPath)
			return fmt.Errorf("download exceeds %d byte limit", limit)
		}
		return nil
	}
	return fmt.Errorf("download failed after %d retries: %w", DefaultMaxRetries, lastErr)
}
