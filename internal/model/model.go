// Пакет model — общие типы, которые ходят между ядром, стором и модулями.
// Сам ни от чего внутри chaff не зависит, поэтому его можно импортировать
// откуда угодно без циклов.
package model

import (
	"net/netip"
	"regexp"
	"strings"
)

// Kind — класс индикатора. Это стабильный контракт: форматы фидов приходят и
// уходят, а виды остаются.
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

// Action — что делаем с подходящим трафиком. allow сильнее block.
type Action string

const (
	ActionBlock   Action = "block"
	ActionMonitor Action = "monitor"
	ActionAllow   Action = "allow"
)

// Scope — весь домен или только конкретные пути. Легит-инфра (github.com) идёт
// как path: домен живой, интересны лишь вредоносные URL.
type Scope string

const (
	ScopeDomain Scope = "domain"
	ScopePath   Scope = "path"
)

// Indicator — строка source of truth.
type Indicator struct {
	ID        int64  `json:"id"`
	Value     string `json:"value"`
	Kind      Kind   `json:"kind"`
	Action    Action `json:"action"`
	Scope     Scope  `json:"scope"`
	Threat    string `json:"threat,omitempty"`
	Note      string `json:"note,omitempty"`
	SourceID  int64  `json:"source_id"`
	FirstSeen int64  `json:"first_seen"` // unix-секунды
	LastSeen  int64  `json:"last_seen"`
	ExpiresAt int64  `json:"expires_at"` // 0 = бессрочно
	Enabled   bool   `json:"enabled"`
}

// SourceSpec — настроенный фид. Адаптер (csv/fstec/text/hosts) — это модуль, а
// сам фид — строка в БД. Один адаптер обслуживает много фидов.
type SourceSpec struct {
	ID        int64          `json:"id"`
	Name      string         `json:"name"`
	Adapter   string         `json:"adapter"`
	URI       string         `json:"uri"`
	ColumnMap map[string]int `json:"column_map,omitempty"` // поле -> номер колонки, для csv
	Enabled   bool           `json:"enabled"`
}

// Ruleset — снапшот желаемого состояния; apply раздаёт его энфорсерам, каждый
// читает свою часть.
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

// AllowSet — исключения; энфорсер сверяется с ним перед дропом, allow выигрывает.
type AllowSet struct {
	IPs     []netip.Prefix
	Domains map[string]bool
}

var (
	reHex64 = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)
	reHex32 = regexp.MustCompile(`^[a-fA-F0-9]{32}$`)
)

// Classify угадывает Kind по сырому значению. Если у фида есть колонка с типом —
// лучше брать её; это запасной путь для голых списков.
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
	// CIDR проверяем до одиночного IP.
	if _, err := netip.ParsePrefix(v); err == nil {
		return KindCIDR
	}
	if _, err := netip.ParseAddr(v); err == nil {
		return KindIP
	}
	// host с путём — это url, иначе голый домен.
	if i := strings.IndexByte(v, '/'); i > 0 {
		return KindURL
	}
	return KindDomain
}

// NormalizeKind переводит произвольный токен типа (из колонки фида) в Kind,
// падая на Classify для незнакомых токенов.
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
