with (import (fetchTarball https://github.com/nixos/nixpkgs/archive/c56a87d8e41ce69bd2af57eb35dcc117a9dfca33.tar.gz) {});

let

 basePackages = [
  go_1_27
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
