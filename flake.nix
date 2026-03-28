{
  description = "muon build environment";
  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
    jailed-agents = {
      url = "github:andersonjoseph/jailed-agents";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };
  outputs =
    {
      self,
      nixpkgs,
      jailed-agents,
      ...
    }:
    let
      system = "x86_64-linux";
      pkgs = import nixpkgs { inherit system; };
    in
    {
      devShells.${system}.default = pkgs.mkShell {
        packages = with pkgs; [
          bashInteractive
          go
          (jailed-agents.lib.${system}.makeJailedOpencode {
            extraPkgs = [
              nodejs
              go
            ];
          })
        ];
      };
    };
}
