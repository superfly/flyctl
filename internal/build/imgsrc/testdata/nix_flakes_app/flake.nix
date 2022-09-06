{
  description = "Vaultwarden";

  inputs.flake-utils.url = "github:numtide/flake-utils";
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/22.05";

  outputs = { self, nixpkgs, flake-utils }:

    let
      baseImage = nixpkgs.legacyPackages.x86_64-linux.dockerTools.pullImage {
        imageName = "amd64/alpine";
        imageDigest = "sha256:1304f174557314a7ed9eddb4eab12fed12cb0cd9809e4c28f29af86979a3c870";
        sha256 = "025s5552l53j5n3ib0jzf500g5rkwn7db6nly90sxzs6h2asdb6p";
        finalImageName = "amd64/alpine";
        finalImageTag = "3.16.2";
      };
      outputs = flake-utils.lib.eachDefaultSystem (system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in rec {
          packages = flake-utils.lib.flattenTree rec {
            vaultwarden = pkgs.vaultwarden;
            default = vaultwarden;
          };
        }
      );
    in
      outputs // (let
        linux = "x86_64-linux";
        pkgs = nixpkgs.legacyPackages.${linux};
      in
        {
          packages = outputs.packages // {
            ${linux} = outputs.packages.${linux} // {docker = pkgs.dockerTools.buildImage {
              name = "vaultwarden";
              fromImage = baseImage;
              contents = outputs.packages.${linux}.vaultwarden;
              config = {
                Cmd = [ "/bin/vaultwarden" ];
              };
            };};};
        });
}
