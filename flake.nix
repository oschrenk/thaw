{
  description = "Thaw - preview what an update would bring, before anything writes a lock";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";

  # Offer prebuilt binaries from the Cachix cache so `nix run` downloads
  # instead of compiling. Consumers are prompted to trust these.
  nixConfig = {
    extra-substituters = [ "https://oschrenk.cachix.org" ];
    extra-trusted-public-keys = [
      "oschrenk.cachix.org-1:3JOMfkq2vFiLw4UsCVwzu8kWFBkuS/3DD5AojcO9pks="
    ];
  };

  outputs =
    { self, nixpkgs }:
    let
      # No VERSION file and no release flow: the commit names the build. A
      # dirty tree has no rev, so it falls back rather than failing the build.
      version = self.shortRev or "dirty";

      # aarch64-darwin is where previews are run and the only system CI fills
      # the cache for. thaw only shells out to nix, so x86_64-linux can join
      # this list the day something in CI needs to run it rather than build it.
      systems = [
        "aarch64-darwin"
      ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
    in
    {
      packages = forAllSystems (pkgs: rec {
        thaw = pkgs.buildGoModule {
          pname = "thaw";
          inherit version;
          src = self;

          # Pins the vendored dependency set. When go.mod changes, set this to
          # lib.fakeHash, run `nix build`, and paste the hash the error prints.
          vendorHash = "sha256-7K17JaXFsjf163g5PXCb5ng2gYdotnZ2IDKk8KFjNj0=";

          # The generated completions ship with the package, so a profile
          # install completes subcommands, flags and --for subjects.
          nativeBuildInputs = [ pkgs.installShellFiles ];
          postInstall = ''
            installShellCompletion --cmd thaw \
              --bash <($out/bin/thaw completion bash) \
              --fish <($out/bin/thaw completion fish) \
              --zsh <($out/bin/thaw completion zsh)
          '';

          ldflags = [
            "-s"
            "-w"
          ];

          meta = {
            description = "Preview what an update would bring, before anything writes a lock";
            homepage = "https://github.com/oschrenk/thaw";
            mainProgram = "thaw";
          };
        };
        default = thaw;
      });

      apps = forAllSystems (pkgs: rec {
        thaw = {
          type = "app";
          program = "${self.packages.${pkgs.stdenv.hostPlatform.system}.thaw}/bin/thaw";
        };
        default = thaw;
      });

      devShells = forAllSystems (pkgs: {
        default = pkgs.mkShell {
          packages = with pkgs; [
            go # go, language
            golangci-lint # go, linter runner
            gopls # go, lsp
          ];
        };
      });
    };
}
