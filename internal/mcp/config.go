package mcp

import "fmt"

// ParseServerConfigs parses the raw mcp_servers YAML block into ServerConfig
// values. The raw map is keyed by server name, each value is a map of settings.
func ParseServerConfigs(raw map[string]any) ([]ServerConfig, error) {
	var configs []ServerConfig
	for name, v := range raw {
		m, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("mcp: server %q: expected map, got %T", name, v)
		}

		cfg := ServerConfig{
			Name:    name,
			Enabled: true,
		}

		if t, ok := m["transport"].(string); ok {
			cfg.Transport = t
		}
		if cmd, ok := m["command"].(string); ok {
			cfg.Command = cmd
		}
		if u, ok := m["url"].(string); ok {
			cfg.URL = u
		}
		if allowed, ok := stringSlice(m["allowed_tools"]); ok {
			cfg.AllowedTools = allowed
		}
		if denied, ok := stringSlice(m["denied_tools"]); ok {
			cfg.DeniedTools = denied
		}

		if args, ok := m["args"].([]any); ok {
			for _, a := range args {
				if s, ok := a.(string); ok {
					cfg.Args = append(cfg.Args, s)
				}
			}
		}

		if env, ok := m["env"].(map[string]any); ok {
			cfg.Env = make(map[string]string, len(env))
			for k, v := range env {
				if s, ok := v.(string); ok {
					cfg.Env[k] = s
				}
			}
		}

		if e, ok := m["enabled"].(bool); ok {
			cfg.Enabled = e
		}

		if err := ValidateServerConfig(cfg); err != nil {
			return nil, err
		}

		configs = append(configs, cfg)
	}
	return configs, nil
}

// ValidateServerConfig checks that a ServerConfig has all required fields.
func ValidateServerConfig(cfg ServerConfig) error {
	if cfg.Name == "" {
		return fmt.Errorf("mcp: server name is required")
	}
	switch cfg.Transport {
	case "stdio":
		if cfg.Command == "" {
			return fmt.Errorf("mcp: server %q: stdio transport requires command", cfg.Name)
		}
	case "sse":
		if cfg.URL == "" {
			return fmt.Errorf("mcp: server %q: sse transport requires url", cfg.Name)
		}
	case "":
		return fmt.Errorf("mcp: server %q: transport is required (stdio or sse)", cfg.Name)
	default:
		return fmt.Errorf("mcp: server %q: unknown transport %q (supported: stdio, sse)", cfg.Name, cfg.Transport)
	}
	return nil
}

func stringSlice(v any) ([]string, bool) {
	items, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		s, ok := item.(string)
		if !ok {
			continue
		}
		out = append(out, s)
	}
	return out, true
}
