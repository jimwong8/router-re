package ha

import (
	"fmt"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/config"
)

type Config struct {
	PeerURL  string   `json:"peer_url"`
	Secret   string   `json:"secret"`
	SyncKeys []string `json:"sync_keys"`
}

func MaskedSecret(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 4 {
		return "****"
	}
	return s[:2] + "****" + s[len(s)-2:]
}

func ValidateSecret(expected, got string) bool {
	return expected != "" && expected == got
}

func Export(st config.Store, keys []string) (map[string]string, error) {
	out := map[string]string{}
	for _, k := range keys {
		parts := strings.SplitN(k, ".", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid key %q", k)
		}
		v, err := st.Get(parts[0], parts[1])
		if err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, nil
}

func Import(st config.Store, payload map[string]string, mode string, syncKeys []string) error {
	if mode == "" {
		mode = "merge"
	}
	if mode != "merge" && mode != "replace" {
		return fmt.Errorf("invalid mode %q", mode)
	}
	if mode == "replace" {
		// unset keys in syncKeys not present in payload
		for _, k := range syncKeys {
			if _, ok := payload[k]; !ok {
				parts := strings.SplitN(k, ".", 2)
				if len(parts) == 2 {
					_ = st.Unset(parts[0], parts[1])
				}
			}
		}
	}
	for k, v := range payload {
		parts := strings.SplitN(k, ".", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid key %q", k)
		}
		if err := st.Set(parts[0], parts[1], v); err != nil {
			return err
		}
	}
	return nil
}
