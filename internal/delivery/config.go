package delivery

import (
	"encoding/json"
	"strings"

	"github.com/gabinante/flywheel/internal/project"
)

func ParseConfig(extra map[string]string) (Config, error) {
	if len(extra) == 0 {
		return Config{}, nil
	}
	raw := strings.TrimSpace(extra[ConfigKey])
	if raw == "" {
		return Config{}, nil
	}
	var cfg Config
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func StoreConfig(pack project.ContextPack, cfg Config) (project.ContextPack, error) {
	extra := pack.Extra
	if extra == nil {
		extra = make(map[string]string)
	}
	if isZeroConfig(cfg) {
		delete(extra, ConfigKey)
		pack.Extra = extra
		return pack, nil
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return pack, err
	}
	extra[ConfigKey] = string(raw)
	pack.Extra = extra
	return pack, nil
}

func isZeroConfig(cfg Config) bool {
	return strings.TrimSpace(cfg.SCM.Provider) == "" &&
		strings.TrimSpace(cfg.Infrastructure.Provider) == "" &&
		(cfg.Infrastructure.FlyIO == nil ||
			(strings.TrimSpace(cfg.Infrastructure.FlyIO.OrganizationSlug) == "" &&
				len(cfg.Infrastructure.FlyIO.Apps) == 0))
}
