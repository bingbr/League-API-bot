package riot

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

type riotRateLimitFile struct {
	RiotRateLimit riotRateLimitConfig `toml:"riot_rate_limit"`
	Defaults      []rateLimitWindowTOML
	Endpoints     []rateLimitEndpointTOML
}

type riotRateLimitConfig struct {
	Defaults  []rateLimitWindowTOML
	Endpoints []rateLimitEndpointTOML
}

type rateLimitWindowTOML struct {
	Requests int    `toml:"requests"`
	Window   string `toml:"window"`
	Burst    int    `toml:"burst"`
}

type rateLimitEndpointTOML struct {
	Prefix string                `toml:"prefix"`
	Path   string                `toml:"path"`
	Limits []rateLimitWindowTOML `toml:"limits"`
}

func ConfigureRateLimitsFromFile(path string) (bool, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return false, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("read rate limit config %q: %w", path, err)
	}

	var fileCfg riotRateLimitFile
	meta, err := toml.Decode(string(data), &fileCfg)
	if err != nil {
		return false, fmt.Errorf("parse rate limit config %q: %w", path, err)
	}
	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, 0, len(undecoded))
		for _, key := range undecoded {
			keys = append(keys, key.String())
		}
		slices.Sort(keys)
		return false, fmt.Errorf("parse rate limit config %q: unknown keys: %s", path, strings.Join(keys, ", "))
	}

	cfg := fileCfg.RiotRateLimit
	if len(cfg.Defaults) == 0 && len(cfg.Endpoints) == 0 {
		cfg = riotRateLimitConfig{
			Defaults:  fileCfg.Defaults,
			Endpoints: fileCfg.Endpoints,
		}
	}

	defaults, endpoints, err := compileRateLimitConfig(cfg)
	if err != nil {
		return false, fmt.Errorf("compile rate limit config %q: %w", path, err)
	}
	if err := applyRateLimitWindows(defaults, endpoints); err != nil {
		return false, fmt.Errorf("apply rate limit config %q: %w", path, err)
	}
	return true, nil
}

func compileRateLimitConfig(cfg riotRateLimitConfig) ([]rateLimitWindow, map[string][]rateLimitWindow, error) {
	defaults, err := tomlWindowsToRuntime(cfg.Defaults, "defaults")
	if err != nil {
		return nil, nil, err
	}

	endpoints := make(map[string][]rateLimitWindow, len(cfg.Endpoints))
	for _, endpoint := range cfg.Endpoints {
		prefix, err := endpointMatcherPrefix(endpoint)
		if err != nil {
			return nil, nil, err
		}

		windows, err := tomlWindowsToRuntime(endpoint.Limits, fmt.Sprintf("endpoint %q", prefix))
		if err != nil {
			return nil, nil, err
		}
		if len(windows) == 0 {
			continue
		}
		endpoints[prefix] = windows
	}
	return defaults, endpoints, nil
}

func tomlWindowsToRuntime(windows []rateLimitWindowTOML, context string) ([]rateLimitWindow, error) {
	if len(windows) == 0 {
		return nil, nil
	}
	out := make([]rateLimitWindow, 0, len(windows))
	for idx, window := range windows {
		duration, err := time.ParseDuration(strings.TrimSpace(window.Window))
		if err != nil {
			return nil, fmt.Errorf("%s window[%d]: parse duration %q: %w", context, idx, window.Window, err)
		}
		out = append(out, rateLimitWindow{
			Requests: window.Requests,
			Window:   duration,
			Burst:    window.Burst,
		})
	}
	return out, nil
}

func endpointMatcherPrefix(endpoint rateLimitEndpointTOML) (string, error) {
	prefix := strings.TrimSpace(endpoint.Prefix)
	if prefix == "" {
		prefix = strings.TrimSpace(endpoint.Path)
	}
	if prefix == "" {
		return "", fmt.Errorf("endpoint rule requires prefix or path")
	}

	// Template paths include "{...}" placeholders.
	if idx := strings.Index(prefix, "{"); idx >= 0 {
		prefix = prefix[:idx]
	}
	prefix = strings.TrimSuffix(prefix, "*")
	prefix = filepath.ToSlash(strings.TrimSpace(prefix))
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	if prefix == "/" {
		return "", fmt.Errorf("endpoint prefix %q is too broad", endpoint.Path)
	}
	return prefix, nil
}
