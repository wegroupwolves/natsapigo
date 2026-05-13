# shell.nix
{pkgs ? import <nixpkgs> {} }:

pkgs.mkShell {
    packages = [
        # DEVELOPMENT
        pkgs.gotest
        pkgs.pre-commit

        # IDE
        pkgs.gotools
        pkgs.air

        # DEPLOYMENT
        pkgs.ko
        pkgs.natscli
    ];

    LD_LIBRARY_PATH = "${pkgs.lib.makeLibraryPath [
        pkgs.stdenv.cc.cc
    ]}";

    shellHook = ''
        set -a; source .env; set +a
        echo "SHELLHOOK LOG: .env loaded to ENV variables"
    '';
}
