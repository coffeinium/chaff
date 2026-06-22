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
	KindSHA256  Kind = "sha256"
	KindMD5     Kind = "md5"
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
	Threat    string `json:"threat,omitempty"`
	Note      string `json:"note,omitempty"`
	SourceID  int64  `json:"source_id"`
	FirstSeen int64  `json:"first_seen"`
	LastSeen  int64  `json:"last_seen"`
	ExpiresAt int64  `json:"expires_at"`
	Enabled   bool   `json:"enabled"`
}

type SourceSpec struct {
	ID        int64          `json:"id"`
	Name      string         `json:"name"`
	Adapter   string         `json:"adapter"`
	URI       string         `json:"uri"`
	ColumnMap map[string]int `json:"column_map,omitempty"`
	Enabled   bool           `json:"enabled"`
}

type Ruleset struct {
	Rev     int64
	IPv4    []netip.Prefix
	IPv6    []netip.Prefix
	Domains []DomainRule
	URLs    []URLRule
	Allow   AllowSet
}

type DomainRule struct {
	Domain string
	Scope  Scope
	Action Action
	Threat string
}

type URLRule struct {
	URL    string
	Action Action
	Threat string
}

type AllowSet struct {
	IPs     []netip.Prefix
	Domains map[string]bool
}

var (
	reHex64 = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)
	reHex32 = regexp.MustCompile(`^[a-fA-F0-9]{32}$`)
)

func Classify(v string) Kind {
	v = strings.TrimSpace(v)
	if v == "" {
		return KindUnknown
	}
	switch {
	case reHex64.MatchString(v):
		return KindSHA256
	case reHex32.MatchString(v):
		return KindMD5
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
	case "sha256":
		return KindSHA256
	case "md5":
		return KindMD5
	}
	return Classify(value)
}
