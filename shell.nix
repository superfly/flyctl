with (import (fetchTarball https://github.com/nixos/nixpkgs/archive/592dc9ed7f049c565e9d7c04a4907e57ae17e2d9.tar.gz) {});

let

 basePackages = [
  go_1_18
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
