{
  description = "Hanji — a markdown-first website framework where the document is the app";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs =
    { self, nixpkgs, ... }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "aarch64-darwin"
      ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
    in
    {
      devShells = forAllSystems (pkgs: {
        default = pkgs.mkShell {
          packages = with pkgs; [
            go_1_27
            gopls
          ];
        };
      });

      # `nix run . -- dev` / `nix build`
      packages = forAllSystems (pkgs: {
        default = pkgs.buildGoModule {
          pname = "hanji";
          version = "0.1.0";
          src = self;
          vendorHash = null; # standard library only
          subPackages = [ "cmd/hanji" ];
        };
      });
    };
}
