package riot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const (
	defaultRateLimitRequests = 1900
	defaultRateLimitBurst    = 50
	defaultRateLimitWindow   = 10 * time.Second
	maxRetryAttempts         = 3
	retryBaseDelay           = time.Second
	retryMaxDelay            = 30 * time.Second
)

var (
	limiterStateMu  = sync.RWMutex{}
	defaultLimiters = []*rate.Limiter{
		newRateLimiter(defaultRateLimitRequests, defaultRateLimitWindow, defaultRateLimitBurst),
	}
	endpointLimiters []endpointLimiter

	rateLimitMu    sync.RWMutex
	rateLimitUntil time.Time
)

func doRiotJSONWithRetry(ctx context.Context, endpoint, apiKey string, target any) error {
	var lastErr error
	for attempt := range maxRetryAttempts {
		if attempt > 0 {
			backoff := min(retryBaseDelay*time.Duration(1<<uint(attempt)), retryMaxDelay)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}

		if err := waitForRateLimit(ctx, endpoint); err != nil {
			return err
		}

		statusErr, err := doRiotRequest(ctx, endpoint, apiKey, target)
		if err == nil && statusErr == nil {
			return nil
		}
		if err != nil {
			if isRetryableRequestError(err) {
				lastErr = err
				continue
			}
			return err
		}
		if !isRetryable(statusErr.StatusCode) {
			return statusErr
		}
		lastErr = statusErr
	}
	return lastErr
}

func waitForRateLimit(ctx context.Context, endpoint string) error {
	rateLimitMu.RLock()
	until := rateLimitUntil
	rateLimitMu.RUnlock()

	if now := time.Now(); now.Before(until) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(until.Sub(now)):
		}
	}

	limiters := limitersForEndpoint(endpoint)
	for _, limiter := range limiters {
		if err := limiter.Wait(ctx); err != nil {
			return err
		}
	}
	return nil
}

func isRetryable(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests ||
		statusCode == http.StatusInternalServerError ||
		statusCode == http.StatusBadGateway ||
		statusCode == http.StatusServiceUnavailable ||
		statusCode == http.StatusGatewayTimeout
}

func isRetryableRequestError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	_, ok := errors.AsType[net.Error](err)
	return ok
}

func doRiotRequest(ctx context.Context, endpoint, apiKey string, target any) (*HTTPStatusError, error) {
	apiKey, err := requireNonEmpty("riot api key", apiKey)
	if err != nil {
		return nil, err
	}
	if target == nil {
		return nil, fmt.Errorf("target is nil")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", defaultRiotUserAgent)
	req.Header.Set("X-Riot-Token", apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request %s: %w", endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
			return nil, fmt.Errorf("decode %s: %w", endpoint, err)
		}
		return nil, nil
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	statusErr := &HTTPStatusError{
		URL:        endpoint,
		StatusCode: resp.StatusCode,
		Body:       string(body),
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		if retryAfter := parseRetryAfter(resp); retryAfter > 0 {
			rateLimitMu.Lock()
			if newUntil := time.Now().Add(retryAfter); newUntil.After(rateLimitUntil) {
				rateLimitUntil = newUntil
			}
			rateLimitMu.Unlock()
		}
	}
	return statusErr, nil
}

func parseRetryAfter(resp *http.Response) time.Duration {
	val := resp.Header.Get("Retry-After")
	if val == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(val); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if t, err := http.ParseTime(val); err == nil {
		return time.Until(t)
	}
	return 0
}

type endpointLimiter struct {
	prefix   string
	limiters []*rate.Limiter
}

type rateLimitWindow struct {
	Requests int
	Window   time.Duration
	Burst    int
}

func applyRateLimitWindows(defaultWindows []rateLimitWindow, endpoints map[string][]rateLimitWindow) error {
	compiledDefaults, err := compileLimiters(defaultWindows)
	if err != nil {
		return fmt.Errorf("compile default limiters: %w", err)
	}
	if len(compiledDefaults) == 0 {
		compiledDefaults = []*rate.Limiter{newRateLimiter(defaultRateLimitRequests, defaultRateLimitWindow, defaultRateLimitBurst)}
	}

	compiledEndpoints := make([]endpointLimiter, 0, len(endpoints))
	for prefix, windows := range endpoints {
		compiled, err := compileLimiters(windows)
		if err != nil {
			return fmt.Errorf("compile endpoint limiter %q: %w", prefix, err)
		}
		if len(compiled) == 0 {
			continue
		}
		compiledEndpoints = append(compiledEndpoints, endpointLimiter{
			prefix:   prefix,
			limiters: compiled,
		})
	}
	sort.Slice(compiledEndpoints, func(i, j int) bool {
		return len(compiledEndpoints[i].prefix) > len(compiledEndpoints[j].prefix)
	})

	limiterStateMu.Lock()
	defaultLimiters = compiledDefaults
	endpointLimiters = compiledEndpoints
	limiterStateMu.Unlock()
	return nil
}

func compileLimiters(windows []rateLimitWindow) ([]*rate.Limiter, error) {
	if len(windows) == 0 {
		return nil, nil
	}
	limiters := make([]*rate.Limiter, 0, len(windows))
	for _, window := range windows {
		requests := window.Requests
		duration := window.Window
		if requests <= 0 || duration <= 0 {
			return nil, fmt.Errorf("invalid limiter window: requests=%d window=%s", requests, duration)
		}
		burst := window.Burst
		if burst <= 0 {
			burst = min(requests, defaultRateLimitBurst)
			if burst <= 0 {
				burst = 1
			}
		}
		limiters = append(limiters, newRateLimiter(requests, duration, burst))
	}
	return limiters, nil
}

func newRateLimiter(requests int, window time.Duration, burst int) *rate.Limiter {
	return rate.NewLimiter(rate.Every(window/time.Duration(requests)), burst)
}

func limitersForEndpoint(endpoint string) []*rate.Limiter {
	path := endpointPath(endpoint)

	limiterStateMu.RLock()
	defer limiterStateMu.RUnlock()

	selected := slices.Clone(defaultLimiters)
	if path == "" || len(endpointLimiters) == 0 {
		return selected
	}
	for _, entry := range endpointLimiters {
		if pathMatchesPrefix(path, entry.prefix) {
			selected = append(selected, entry.limiters...)
			break
		}
	}
	return selected
}

func endpointPath(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Path
}

func pathMatchesPrefix(path, prefix string) bool {
	if path == "" || prefix == "" {
		return false
	}
	if strings.HasSuffix(prefix, "/") {
		return strings.HasPrefix(path, prefix)
	}
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}
