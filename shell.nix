with (import (fetchTarball https://github.com/nixos/nixpkgs/archive/779db7898ca58416f6d1e5bc3c4ed4dafd32879c.tar.gz) {});

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
