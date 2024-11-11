package runtime

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// Built-in health checkers
type HTTPHealthChecker struct {
	URL     string
	Timeout time.Duration
}

func (h *HTTPHealthChecker) Check(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", h.URL, nil)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: h.Timeout}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unhealthy status code: %d", resp.StatusCode)
	}

	return nil
}

func (r *Runtime) monitorHealth(ctx context.Context, name string, proc *managedProcess) {
	if proc.health.checker == nil {
		return
	}

	ticker := time.NewTicker(proc.health.checker.(*HTTPHealthChecker).Timeout)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			checkCtx, cancel := context.WithTimeout(ctx, proc.health.checker.(*HTTPHealthChecker).Timeout)
			err := proc.health.checker.Check(checkCtx)
			cancel()

			if err != nil {
				proc.health.failures++
				proc.health.status = fmt.Sprintf("unhealthy: %v", err)

				// Check if we need to restart
				if proc.health.failures >= proc.health.maxFailures {
					r.logger.Error("Health check failed, restarting process",
						"name", name,
						"failures", proc.health.failures)

					// Restart the process
					proc.cancel()
					// Start will be handled by the monitor goroutine
				}
			} else {
				proc.health.failures = 0
				proc.health.status = "healthy"
			}

			proc.health.lastCheck = time.Now()

		case <-ctx.Done():
			return
		}
	}
}
