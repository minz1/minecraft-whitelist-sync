package whitelistsync

import "time"

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

	Debounce          time.Duration
	MinInterval       time.Duration
	ReconcileInterval time.Duration
	HTTPTimeout       time.Duration
}
