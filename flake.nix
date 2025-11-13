{
  description = "ytplv Go web service";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-24.05";

  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAllSystems = nixpkgs.lib.genAttrs systems;
    in {
      packages = forAllSystems (system:
        let
          pkgs = import nixpkgs { inherit system; };
        in {
          default = pkgs.buildGoModule {
            pname = "ytplv-server";
            version = "unstable";
            src = self;
            subPackages = [ "cmd/server" ];
            vendorHash = pkgs.lib.fakeHash; # replace via `nix build` error output
            ldflags = [ "-s" "-w" ];
          };
        });

      devShells = forAllSystems (system:
        let
          pkgs = import nixpkgs { inherit system; };
        in {
          default = pkgs.mkShell {
            packages = with pkgs; [ go gopls gotools gotestsum ];
            shellHook = ''
              echo "Dev shell ready. Run: go test ./... && go run ./cmd/server"
            '';
          };
        });

      checks = forAllSystems (system:
        let
          pkgs = import nixpkgs { inherit system; };
        in {
          unit = pkgs.stdenv.mkDerivation {
            name = "go-tests";
            src = self;
            nativeBuildInputs = [ pkgs.go ];
            buildPhase = "go test ./...";
            installPhase = ''
              mkdir -p $out
              echo ok > $out/result
            '';
          };
        });
    };
}

