{
  description = "minecraft-whitelist-sync — syncs a Minecraft server's whitelist from Authentik";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-parts.url = "github:hercules-ci/flake-parts";
  };

  outputs =
    inputs@{ flake-parts, ... }:
    flake-parts.lib.mkFlake { inherit inputs; } {
      systems = [
        "x86_64-linux"
        "aarch64-linux"
      ];

      perSystem =
        { pkgs, ... }:
        let
          minecraft-whitelist-sync = pkgs.buildGo127Module {
            pname = "minecraft-whitelist-sync";
            version = "0.1.0";
            src = ./.;
            vendorHash = null;
            env.CGO_ENABLED = "0";
            ldflags = [
              "-s"
              "-w"
            ];
            meta = {
              description = "Syncs a Minecraft server's whitelist from Authentik's user directory";
              mainProgram = "minecraft-whitelist-sync";
            };
          };
        in
        {
          packages = {
            default = minecraft-whitelist-sync;
            inherit minecraft-whitelist-sync;
          };

          devShells.default = pkgs.mkShell {
            buildInputs = with pkgs; [
              go_1_27
              gopls
              gotools
              go-tools
              golangci-lint
            ];
          };
        };

      flake = {
        nixosModules.default = import ./nix/module.nix;
        nixosModules.minecraft-whitelist-sync = import ./nix/module.nix;
      };
    };
}
