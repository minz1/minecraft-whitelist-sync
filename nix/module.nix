{
  config,
  lib,
  pkgs,
  ...
}:
let
  cfg = config.services.minecraft-whitelist-sync;
in
{
  options.services.minecraft-whitelist-sync = {
    enable = lib.mkEnableOption "minecraft-whitelist-sync";

    package = lib.mkPackageOption pkgs "minecraft-whitelist-sync" { };

    listenAddr = lib.mkOption {
      type = lib.types.str;
      default = ":8765";
      description = "Listen address for the webhook HTTP server (LISTEN_ADDR).";
    };

    environmentFile = lib.mkOption {
      type = lib.types.path;
      description = ''
        Path to a file containing environment variables loaded by systemd.
        Expected variables: RCON_HOST, RCON_PORT, RCON_PASSWORD, AUTHENTIK_URL,
        AUTHENTIK_TOKEN, AUTHENTIK_CLIENT_CERT, AUTHENTIK_CLIENT_KEY,
        WHITELIST_FILE, WEBHOOK_TOKEN, and optionally DEBOUNCE, MIN_INTERVAL,
        RECONCILE_INTERVAL.

        With sops-nix, set this to config.sops.templates."whitelist-sync-env".path.
      '';
    };

    extraServiceConfig = lib.mkOption {
      type = lib.types.attrs;
      default = { };
      description = ''
        Extra systemd serviceConfig attrs, merged over this module's own
        defaults. Left to the consuming host so it can apply its own
        hardening (e.g. mkHardened) rather than this module hardcoding one.
      '';
    };
  };

  config = lib.mkIf cfg.enable {
    systemd.services.minecraft-whitelist-sync = {
      description = "Sync Authentik minecraft users to the whitelist on webhook/reconcile";
      wantedBy = [ "multi-user.target" ];
      after = [ "network-online.target" ];
      wants = [ "network-online.target" ];

      serviceConfig = {
        ExecStart = lib.getExe cfg.package;
        EnvironmentFile = cfg.environmentFile;
        Environment = [ "LISTEN_ADDR=${cfg.listenAddr}" ];
        Restart = "on-failure";
        RestartSec = "5s";
      }
      // cfg.extraServiceConfig;
    };
  };
}
