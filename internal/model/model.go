package model

import (
	"net/netip"
	"regexp"
	"strings"
)

type Kind string

const (
	KindIP      Kind = "ip"
	KindCIDR    Kind = "cidr"
	KindDomain  Kind = "domain"
	KindURL     Kind = "url"
	KindMAC     Kind = "mac"
	KindUnknown Kind = ""
)

type Action string

const (
	ActionBlock   Action = "block"
	ActionMonitor Action = "monitor"
	ActionAllow   Action = "allow"
)

type Scope string

const (
	ScopeDomain Scope = "domain"
	ScopePath   Scope = "path"
)

type Indicator struct {
	ID        int64  `json:"id"`
	Value     string `json:"value"`
	Kind      Kind   `json:"kind"`
	Action    Action `json:"action"`
	Scope     Scope  `json:"scope"`
	Note      string `json:"note,omitempty"`
	SourceID  int64  `json:"source_id"`
	FirstSeen int64  `json:"first_seen"`
	LastSeen  int64  `json:"last_seen"`
	ExpiresAt int64  `json:"expires_at"`
	Enabled   bool   `json:"enabled"`
}

type SourceSpec struct {
	ID         int64          `json:"id"`
	Name       string         `json:"name"`
	Adapter    string         `json:"adapter"`
	URI        string         `json:"uri"`
	ColumnMap  map[string]int `json:"column_map,omitempty"`
	Enabled    bool           `json:"enabled"`
	LastSync   int64          `json:"last_sync,omitempty"`
	LastStatus string         `json:"last_status,omitempty"`
	LastCount  int            `json:"last_count,omitempty"`
}

type Ruleset struct {
	Rev     int64
	IPv4    []netip.Prefix
	IPv6    []netip.Prefix
	MACs    []string
	Domains []DomainRule
	URLs    []URLRule
	Allow   AllowSet
	Groups  []GroupPolicy
}

// GroupPolicy — правила, действующие только на машины группы (по их MAC).
// Глобальные правила всегда приоритетнее групповых.
type GroupPolicy struct {
	ID      int64
	Name    string
	MACs    []string
	IPv4    []netip.Prefix
	Domains []DomainRule
	URLs    []URLRule
	Allow   AllowSet
}

type DomainRule struct {
	Domain string
	Scope  Scope
	Action Action
}

type URLRule struct {
	URL    string
	Action Action
}

type AllowSet struct {
	IPs     []netip.Prefix
	Domains map[string]bool
}

var (
	reHex64 = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)
	reHex32 = regexp.MustCompile(`^[a-fA-F0-9]{32}$`)
	reMAC   = regexp.MustCompile(`^[a-fA-F0-9]{2}([:-][a-fA-F0-9]{2}){5}$`)
)

func Classify(v string) Kind {
	v = strings.TrimSpace(v)
	if v == "" {
		return KindUnknown
	}
	switch {
	case reMAC.MatchString(v):
		return KindMAC
	case reHex64.MatchString(v), reHex32.MatchString(v):
		return KindUnknown
	}
	if strings.Contains(v, "://") {
		return KindURL
	}

	if _, err := netip.ParsePrefix(v); err == nil {
		return KindCIDR
	}
	if _, err := netip.ParseAddr(v); err == nil {
		return KindIP
	}

	if i := strings.IndexByte(v, '/'); i > 0 {
		return KindURL
	}
	return KindDomain
}

func NormalizeKind(token, value string) Kind {
	switch strings.ToLower(strings.TrimSpace(token)) {
	case "ip", "ipv4", "ipv6", "ip-dst", "ip-src":
		return KindIP
	case "cidr", "net", "network":
		return KindCIDR
	case "domain", "hostname", "fqdn":
		return KindDomain
	case "url", "uri", "link":
		return KindURL
	case "mac", "mac-src", "ether":
		return KindMAC
	case "sha256", "md5", "sha1", "hash", "filehash":
		return KindUnknown
	}
	return Classify(value)
}

func NormalizeMAC(v string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(v), "-", ":"))
}
