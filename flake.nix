{
  description = "castweb";

  # Use a nixpkgs with Go >= 1.24.6
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs =
    { self, nixpkgs }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];
      forAllSystems = nixpkgs.lib.genAttrs systems;
    in
    {
      packages = forAllSystems (
        system:
        let
          pkgs = import nixpkgs { inherit system; };
        in
        {
          default = pkgs.buildGoModule {
            pname = "ytplv-server";
            version = "unstable";
            # Use local working tree to include untracked files during development
            src = ./.;
            subPackages = [ "cmd/castweb" ];
            # No external modules; disable vendoring
            vendorHash = null;
            ldflags = [
              "-s"
              "-w"
            ];
            # pin Go toolchain
            go = pkgs.go_1_24;
            # Ensure ytcast is available at runtime by wrapping the binary
            nativeBuildInputs = [ pkgs.makeWrapper ];
            postInstall = ''
              wrapProgram "$out/bin/castweb" \
                --prefix PATH : ${pkgs.lib.makeBinPath [ pkgs.ytcast ]}
            '';
          };
        }
      );

      devShells = forAllSystems (
        system:
        let
          pkgs = import nixpkgs { inherit system; };
        in
        {
          default = pkgs.mkShell {
            packages = with pkgs; [
              go_1_24
              gopls
              gotools
              gotestsum
              ytcast
            ];
            shellHook = ''
              echo "Dev shell ready. Run: go test ./... && go run ./cmd/castweb -root ./testdata -port 8080"
            '';
          };
        }
      );

      checks = forAllSystems (
        system:
        let
          pkgs = import nixpkgs { inherit system; };
        in
        {
          unit = pkgs.stdenv.mkDerivation {
            name = "go-tests";
            # Use local working tree for tests too
            src = ./.;
            nativeBuildInputs = [ pkgs.go_1_24 ];
            buildPhase = "go test ./...";
            installPhase = ''
              mkdir -p $out
              echo ok > $out/result
            '';
          };
        }
      );
    };
}
