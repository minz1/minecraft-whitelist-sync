// Package whitelistsync keeps a Minecraft server's whitelist in sync with
// Authentik's user directory. It re-fetches the full user list on every
// trigger — a webhook, a periodic reconcile tick, or startup — diffs it
// against whitelist.json, and reloads the running server over RCON.
package whitelistsync

import "time"

// Config holds everything a Syncer needs. All fields are required unless
// noted otherwise.
type Config struct {
	RCONHost     string
	RCONPort     string
	RCONPassword string
	RCONTimeout  time.Duration

	AuthentikURL   string
	AuthentikToken string
	ClientCertPath string
	ClientKeyPath  string

	WhitelistFile string
	WebhookToken  string

	// Debounce coalesces bursts of triggers before syncing.
	Debounce time.Duration
	// MinInterval enforces a minimum gap between completed syncs.
	MinInterval time.Duration
	// ReconcileInterval is the periodic safety-net sync interval.
	ReconcileInterval time.Duration
	// HTTPTimeout bounds each call to the Authentik API.
	HTTPTimeout time.Duration
}
