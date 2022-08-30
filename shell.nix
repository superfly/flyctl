with (import (fetchTarball https://github.com/nixos/nixpkgs/archive/3402d9c4a4fe77e245c1b3b061997a83e6f7504e.tar.gz) {});

let

 basePackages = [
  go_1_19
  gnumake
  gnused
  ];

#  inputs = basePackages
#    ++ lib.optionals stdenv.isDarwin (with darwin.apple_sdk.frameworks; [
#        Security
#      ]);

in mkShell {
  buildInputs = basePackages;

}
