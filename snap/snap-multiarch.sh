#!/bin/bash
#
# Build the ipfs-cluster snaps and push them to the store.

set -ev

snap() {
    snapcraft clean
    ARCH_TRIPLET=$1 TARGET_GOARCH=$2 snapcraft --target-arch $3
    snapcraft push ipfs-cluster*$3.snap --release edge
}

snap x86_64-linux-gnu amd64 amd64
snap arm-linux-gnueabihf arm armhf
snap aarch64-linux-gnu arm64 arm64
