#!/usr/bin/env bash

echo "Building binaries for various platforms"

# Create directory to store all binaries
binsdir=/tmp/mgbins
mkdir -p $binsdir
echo "Output directory for binaries created: $binsdir"

# OS and architecture matrix
oses="linux darwin windows"
arcs="386 amd64"

# Build binaries for all OSes and architectures
for os in $oses; do
  for arc in $arcs; do
    echo "Building binary for $os/$arc"
    fn=$binsdir/memgator-$os-$arc
    if [ $os == "windows" ]; then
      GOOS=$os GOARCH=$arc go build -v -o $fn.exe
    else
      GOOS=$os GOARCH=$arc CGO_ENABLED=0 go build -v -a -installsuffix cgo -o $fn
    fi
  done
done

echo "Following binaries have been created in: $binsdir"
ls -lh $binsdir

exit 0
