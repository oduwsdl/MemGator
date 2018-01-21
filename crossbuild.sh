#!/usr/bin/env bash

echo "Building binaries for various platforms"

# Create directory to store all binaries
binsdir=/tmp/mgbins
mkdir -p $binsdir

# Build binaries for Mac and Linux
for GOOS in darwin linux; do
  for GOARCH in 386 amd64; do
    export GOOS=$GOOS
    export GOARCH=$GOARCH
    export CGO_ENABLED=0
    go build -v -a -installsuffix cgo -o $binsdir/memgator-$GOOS-$GOARCH
  done
done

# Build binaries for Windows
for GOOS in windows; do
  for GOARCH in 386 amd64; do
    export GOOS=$GOOS
    export GOARCH=$GOARCH
    go build -v -o $binsdir/memgator-$GOOS-$GOARCH.exe
  done
done

# Copy the static binary to the bin directory of the repo
cp $binsdir/memgator-linux-amd64 ./bin/memgator

echo "Following binaries have been created in $binsdir"
ls -l $binsdir

exit 0
