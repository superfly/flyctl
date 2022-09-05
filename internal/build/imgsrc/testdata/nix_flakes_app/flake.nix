{
  description = "Vaultwarden";

  inputs.flake-utils.url = "github:numtide/flake-utils";
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/22.05";

  outputs = { self, nixpkgs, flake-utils }:

    let
      baseImages = builtins.listToAttrs(builtins.map (system:
        # For now, we use the same base image for all since
        # Mac will have linux emulation
        {name = system; value = nixpkgs.legacyPackages.${system}.dockerTools.pullImage {
          imageName = "amd64/alpine";
          imageDigest = "sha256:1304f174557314a7ed9eddb4eab12fed12cb0cd9809e4c28f29af86979a3c870";
          sha256 = "025s5552l53j5n3ib0jzf500g5rkwn7db6nly90sxzs6h2asdb6p";
          finalImageName = "amd64/alpine";
          finalImageTag = "3.16.2";
         };
        }
      ) flake-utils.lib.defaultSystems);
    in

      flake-utils.lib.eachDefaultSystem (system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
          linuxPkgs = nixpkgs.legacyPackages.x86_64-linux;

        in rec {
          packages = flake-utils.lib.flattenTree rec {
            default = pkgs.vaultwarden;

            docker = pkgs.dockerTools.buildImage {
              name = "vaultwarden";
              fromImage = baseImages.${system};
              contents = linuxPkgs.vaultwarden;
              config = {
                Cmd = [ "/bin/vaultwarden" ];
              };
            };
          };
        }
      );
}
