package system_setting

import (
	"fmt"
	"net/netip"
	"strings"
	"sync/atomic"

	"github.com/QuantumNous/new-api/setting/config"
)

type ClientIPSetting struct {
	BlacklistEnabled bool     `json:"blacklist_enabled"`
	Blacklist        []string `json:"blacklist"`
	TrustedProxies   []string `json:"trusted_proxies"`
}

type ClientIPSnapshot struct {
	BlacklistEnabled bool
	Blacklist        []netip.Prefix
	TrustedProxies   []netip.Prefix
}

var defaultClientIPSetting = ClientIPSetting{
	BlacklistEnabled: false,
	Blacklist:        []string{},
	TrustedProxies:   []string{},
}

var clientIPSnapshot atomic.Value

func init() {
	snapshot, err := compileClientIPSetting(defaultClientIPSetting)
	if err != nil {
		panic(err)
	}
	clientIPSnapshot.Store(snapshot)
	config.GlobalConfig.Register("client_ip_setting", &defaultClientIPSetting)
}

func GetClientIPSetting() ClientIPSetting {
	return ClientIPSetting{
		BlacklistEnabled: defaultClientIPSetting.BlacklistEnabled,
		Blacklist:        append([]string(nil), defaultClientIPSetting.Blacklist...),
		TrustedProxies:   append([]string(nil), defaultClientIPSetting.TrustedProxies...),
	}
}

func GetClientIPSnapshot() ClientIPSnapshot {
	snapshot := clientIPSnapshot.Load().(ClientIPSnapshot)
	return ClientIPSnapshot{
		BlacklistEnabled: snapshot.BlacklistEnabled,
		Blacklist:        snapshot.Blacklist,
		TrustedProxies:   snapshot.TrustedProxies,
	}
}

func ValidateClientIPSetting(value ClientIPSetting) (ClientIPSnapshot, error) {
	return compileClientIPSetting(value)
}

func UpdateAndSyncClientIPSetting() error {
	snapshot, err := compileClientIPSetting(GetClientIPSetting())
	if err != nil {
		return err
	}
	clientIPSnapshot.Store(snapshot)
	return nil
}

func IsClientIPSettingKey(key string) bool {
	switch key {
	case "client_ip_setting.blacklist_enabled", "client_ip_setting.blacklist", "client_ip_setting.trusted_proxies":
		return true
	default:
		return false
	}
}

func compileClientIPSetting(value ClientIPSetting) (ClientIPSnapshot, error) {
	blacklist, err := normalizeIPPrefixes("blacklist", value.Blacklist)
	if err != nil {
		return ClientIPSnapshot{}, err
	}
	trustedProxies, err := normalizeIPPrefixes("trusted proxy", value.TrustedProxies)
	if err != nil {
		return ClientIPSnapshot{}, err
	}
	return ClientIPSnapshot{
		BlacklistEnabled: value.BlacklistEnabled,
		Blacklist:        blacklist,
		TrustedProxies:   trustedProxies,
	}, nil
}

func normalizeIPPrefixes(kind string, rules []string) ([]netip.Prefix, error) {
	prefixes := make([]netip.Prefix, 0, len(rules))
	seen := make(map[netip.Prefix]struct{}, len(rules))
	for _, rule := range rules {
		rule = strings.TrimSpace(rule)
		if rule == "" {
			continue
		}

		prefix, err := netip.ParsePrefix(rule)
		if err != nil {
			addr, addrErr := netip.ParseAddr(rule)
			if addrErr != nil {
				return nil, fmt.Errorf("invalid %s rule %q", kind, rule)
			}
			addr = addr.Unmap()
			prefix = netip.PrefixFrom(addr, addr.BitLen())
		} else {
			addr := prefix.Addr().Unmap()
			bits := prefix.Bits()
			if addr.Is4() && bits > 32 {
				bits -= 96
			}
			prefix = netip.PrefixFrom(addr, bits)
		}

		prefix = prefix.Masked()
		if _, ok := seen[prefix]; ok {
			continue
		}
		seen[prefix] = struct{}{}
		prefixes = append(prefixes, prefix)
	}
	return prefixes, nil
}
