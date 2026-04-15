{
  description = "eyesonly - secure secret sharing";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/ab7b6889ae9d484eed2876868209e33eb262511d";
    nixpkgs-go.url = "github:NixOS/nixpkgs/nixos-25.11";
    nixpkgs-act.url = "github:NixOS/nixpkgs/13043924aaa7375ce482ebe2494338e058282925";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, nixpkgs-go, nixpkgs-act, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
        go-pkgs = nixpkgs-go.legacyPackages.${system};
        act-pkg = nixpkgs-act.legacyPackages.${system}.act;
      in
      {
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            gopls
            gotools
            go-tools
            sqlite
          ] ++ [ go-pkgs.go_1_26 act-pkg ];

          shellHook = ''
            export GO111MODULE=on
            echo "$(go version)"
            echo "$(act --version)"
          '';
        };
      }
    );
}
