{
  description = "eyesonly - secure secret sharing";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-25.11";
    nixpkgs-act.url = "github:NixOS/nixpkgs/13043924aaa7375ce482ebe2494338e058282925";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, nixpkgs-act, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
        act-pkg = nixpkgs-act.legacyPackages.${system}.act;
      in
      {
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go_1_26
            apple-sdk_15
            gopls
            gotools
            go-tools
            sqlite
          ] ++ [ act-pkg ];

          shellHook = ''
            export GO111MODULE=on
            echo "$(go version)"
            echo "$(act --version)"
          '';
        };
      }
    );
}
