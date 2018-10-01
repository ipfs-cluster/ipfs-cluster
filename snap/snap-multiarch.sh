#!/bin/bash
#
# Build the ipfs-cluster snaps and push them to the store.

set -ev

release=$1

snap() {
    export SNAP_ARCH_TRIPLET=$1
    export TARGET_GOARCH=$2
    target_arch=$3
    release=$4
    echo "Building and pushing snap:"
    echo "SNAP_ARCH_TRIPLET=${SNAP_ARCH_TRIPLET}"
    echo "TARGET_GOARCH=${TARGET_GOARCH}"
    echo "target_arch=${target_arch}"
    echo "release=${release}"

    snapcraft clean
    snapcraft --target-arch $target_arch
    snapcraft push ipfs-cluster*${target_arch}.snap --release $release
}

snap x86_64-linux-gnu amd64 amd64 $release
snap arm-linux-gnueabihf arm armhf $release
snap aarch64-linux-gnu arm64 arm64 $release
