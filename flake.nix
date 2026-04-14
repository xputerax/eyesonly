{
  description = "eyesonly - secure secret sharing";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/ab7b6889ae9d484eed2876868209e33eb262511d";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go_1_23
            gopls
            gotools
            go-tools
            sqlite
          ];

          shellHook = ''
            export GO111MODULE=on
            echo "Go $(go version) is ready!"
          '';
        };
      }
    );
}
