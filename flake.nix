{
  description = "kraweb - collection of convenience funcs for go http";

  inputs = {
    nixpkgs.url = "nixpkgs/nixpkgs-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = {
    nixpkgs,
    flake-utils,
    ...
  }:
    flake-utils.lib.eachDefaultSystem
    (system: let
      pkgs = import nixpkgs {
        inherit system;
      };
    in {
      # `nix develop`
      devShell = pkgs.mkShell {
        buildInputs = with pkgs; [
          golangci-lint
          git
          go_1_22
        ];
      };
    });
}
