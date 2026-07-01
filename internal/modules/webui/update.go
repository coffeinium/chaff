package webui

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coffeinium/chaff/internal/ipc"
	"github.com/coffeinium/chaff/internal/version"
)

const (
	releaseAPI = "https://api.github.com/repos/coffeinium/chaff/releases/latest"
	repoURL    = "https://github.com/coffeinium/chaff"
)

type updater struct {
	mu     sync.Mutex
	latest string
	url    string
}

func (u *updater) snapshot() (string, string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.url == "" {
		return u.latest, repoURL
	}
	return u.latest, u.url
}

func (u *updater) refresh(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releaseAPI, nil)
	if err != nil {
		return
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "chaff/"+version.Version)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}
	var rel struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&rel); err != nil {
		return
	}
	if rel.TagName == "" {
		return
	}
	u.mu.Lock()
	u.latest, u.url = rel.TagName, rel.HTMLURL
	u.mu.Unlock()
}

func versionHandler(u *updater) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		latest, url := u.snapshot()
		writeResp(w, http.StatusOK, ipc.OK(map[string]any{
			"version":  version.Version,
			"author":   version.Author,
			"latest":   latest,
			"outdated": outdated(version.Version, latest),
			"url":      url,
		}))
	}
}

func outdated(current, latest string) bool {
	c, ok1 := parseSemver(current)
	l, ok2 := parseSemver(latest)
	if !ok1 || !ok2 {
		return false
	}
	for i := 0; i < 3; i++ {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

func parseSemver(s string) ([3]int, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	var out [3]int
	if s == "" {
		return out, false
	}
	for i, p := range strings.SplitN(s, ".", 3) {
		n, err := strconv.Atoi(p)
		if err != nil {
			return out, false
		}
		out[i] = n
	}
	return out, true
}
