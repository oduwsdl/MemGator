#!/usr/bin/env bash

set -e

echo "Building binaries for various platforms"

# Create directory to store all binaries
binsdir=/tmp/mgbins
mkdir -p $binsdir
rm -rf $binsdir/memgator-*
echo "Output directory for binaries created: $binsdir"

# Platforms (OS/arch matrix)
platforms="linux/amd64 darwin/amd64 windows/amd64"

# Build binaries for all listed platforms
for p in $platforms; do
  os=${p%/*}
  arc=${p#*/}
  echo "Building binary for platform: $p"
  fn=$binsdir/memgator-$os-$arc
  if [ $os == "windows" ]; then
    GOOS=$os GOARCH=$arc go build -v -o $fn.exe
  else
    GOOS=$os GOARCH=$arc CGO_ENABLED=0 go build -v -a -installsuffix cgo -o $fn
  fi
done

echo "Following binaries have been created in: $binsdir"
ls -lh $binsdir
