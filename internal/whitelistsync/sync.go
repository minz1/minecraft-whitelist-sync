package whitelistsync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
)

type whitelistEntry struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
}

type authentikUsersPage struct {
	Pagination struct {
		TotalPages int `json:"total_pages"`
	} `json:"pagination"`
	Results []struct {
		Attributes map[string]any `json:"attributes"`
	} `json:"results"`
}

func fetchDesired(ctx context.Context, client *http.Client, cfg Config) (map[string]string, error) {
	desired := make(map[string]string)
	page := 1
	for {
		data, fetchErr := fetchUsersPage(ctx, client, cfg, page)
		if fetchErr != nil {
			return nil, fetchErr
		}
		for _, u := range data.Results {
			uuid, _ := u.Attributes["minecraft_uuid"].(string)
			name, _ := u.Attributes["minecraft_username"].(string)
			if uuid != "" && name != "" {
				desired[uuid] = name
			}
		}
		if page >= data.Pagination.TotalPages {
			return desired, nil
		}
		page++
	}
}

func fetchUsersPage(ctx context.Context, client *http.Client, cfg Config, page int) (*authentikUsersPage, error) {
	base := strings.TrimRight(cfg.AuthentikURL, "/")
	url := fmt.Sprintf("%s/api/v3/core/users/?page_size=100&type=internal&page=%d", base, page)
	req, buildErr := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if buildErr != nil {
		return nil, fmt.Errorf("build request: %w", buildErr)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.AuthentikToken)

	resp, doErr := client.Do(req)
	if doErr != nil {
		return nil, fmt.Errorf("fetch users page %d: %w", page, doErr)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch users page %d: unexpected status %s", page, resp.Status)
	}

	var data authentikUsersPage
	if decodeErr := json.NewDecoder(resp.Body).Decode(&data); decodeErr != nil {
		return nil, fmt.Errorf("decode users page %d: %w", page, decodeErr)
	}
	return &data, nil
}

func readCurrent(path string) map[string]string {
	raw, readErr := os.ReadFile(path)
	if readErr != nil {
		return map[string]string{}
	}
	var entries []whitelistEntry
	if unmarshalErr := json.Unmarshal(raw, &entries); unmarshalErr != nil {
		return map[string]string{}
	}
	current := make(map[string]string, len(entries))
	for _, e := range entries {
		current[e.UUID] = e.Name
	}
	return current
}

func writeWhitelist(path string, desired map[string]string) error {
	entries := make([]whitelistEntry, 0, len(desired))
	for uuid, name := range desired {
		entries = append(entries, whitelistEntry{UUID: uuid, Name: name})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].UUID < entries[j].UUID })

	out, marshalErr := json.MarshalIndent(entries, "", "  ")
	if marshalErr != nil {
		return fmt.Errorf("marshal whitelist: %w", marshalErr)
	}
	return os.WriteFile(path, out, 0o600)
}

func diffSummary(desired, current map[string]string) ([]string, []string, []string) {
	var added, removed, renamed []string
	for uuid, name := range desired {
		if _, ok := current[uuid]; !ok {
			added = append(added, name)
		} else if current[uuid] != name {
			renamed = append(renamed, fmt.Sprintf("%s -> %s", current[uuid], name))
		}
	}
	for uuid, name := range current {
		if _, ok := desired[uuid]; !ok {
			removed = append(removed, name)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	sort.Strings(renamed)
	return added, removed, renamed
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func (s *Syncer) Sync(ctx context.Context) error {
	desired, fetchErr := fetchDesired(ctx, s.client, s.cfg)
	if fetchErr != nil {
		return fetchErr
	}
	current := readCurrent(s.cfg.WhitelistFile)

	if mapsEqual(desired, current) {
		s.log.InfoContext(ctx, "whitelist up to date")
		return nil
	}

	added, removed, renamed := diffSummary(desired, current)
	for _, name := range added {
		s.log.InfoContext(ctx, "adding to whitelist", "name", name)
	}
	for _, name := range removed {
		s.log.InfoContext(ctx, "removing from whitelist", "name", name)
	}
	for _, r := range renamed {
		s.log.InfoContext(ctx, "renaming on whitelist", "change", r)
	}

	if writeErr := writeWhitelist(s.cfg.WhitelistFile, desired); writeErr != nil {
		return fmt.Errorf("write %s: %w", s.cfg.WhitelistFile, writeErr)
	}
	s.log.InfoContext(ctx, "wrote whitelist", "path", s.cfg.WhitelistFile)

	rconAddr := fmt.Sprintf("%s:%s", s.cfg.RCONHost, s.cfg.RCONPort)
	if reloadErr := rconReloadWhitelist(ctx, rconAddr, s.cfg.RCONPassword, s.cfg.RCONTimeout); reloadErr != nil {
		if unavailable, ok := errors.AsType[*rconUnavailableError](reloadErr); ok {
			s.log.WarnContext(ctx, "whitelist reloads on next server start", "error", unavailable)
			return nil
		}
		return fmt.Errorf("rcon reload: %w", reloadErr)
	}
	s.log.InfoContext(ctx, "whitelist reloaded via RCON")
	return nil
}
