# minecraft-whitelist-sync

Keeps a Minecraft server's whitelist in sync with an Authentik user directory. Syncs on startup, on a POST to `/whitelist/notify`, and on a periodic reconcile tick. Each sync re-fetches the full internal user list, diffs it against `whitelist.json`, rewrites the file if it changed, and reloads the running server over RCON.

Users are matched by two Authentik user attributes: `minecraft_uuid` and `minecraft_username`. Both must be set or the user is skipped.

## Config

All via environment variables:

| Variable | Required | Default | Notes |
|---|---|---|---|
| `RCON_HOST` | yes | | |
| `RCON_PORT` | no | `25575` | |
| `RCON_PASSWORD` | yes | | |
| `AUTHENTIK_URL` | yes | | base URL, e.g. `https://authentik.internal:9443` |
| `AUTHENTIK_TOKEN` | yes | | bearer token, needs `authentik_core.view_user` |
| `AUTHENTIK_CLIENT_CERT` | yes | | client cert for mTLS to Authentik |
| `AUTHENTIK_CLIENT_KEY` | yes | | re-read from disk on every TLS handshake, so a renewed cert takes effect without a restart |
| `WHITELIST_FILE` | no | `/persist/atm10/whitelist.json` | |
| `LISTEN_ADDR` | no | `:8765` | |
| `WEBHOOK_TOKEN` | yes | | bearer token required on `POST /whitelist/notify` |
| `DEBOUNCE` | no | `5s` | coalesces bursts of triggers before syncing |
| `MIN_INTERVAL` | no | `30s` | minimum gap between completed syncs |
| `RECONCILE_INTERVAL` | no | `15m` | periodic safety-net sync |

## NixOS module

```nix
{
  inputs.whitelist-sync.url = "github:minz1/minecraft-whitelist-sync";
}
```

```nix
services.minecraft-whitelist-sync = {
  enable = true;
  environmentFile = config.sops.templates."whitelist-sync-env".path;
};
```

## Development

```
go build ./...
go test ./... -race
golangci-lint run ./...
```
