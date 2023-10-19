with (import (fetchTarball https://github.com/nixos/nixpkgs/archive/db9208ab987cdeeedf78ad9b4cf3c55f5ebd269b.tar.gz) {});

let

 basePackages = [
  go_1_21
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
