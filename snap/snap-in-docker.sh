#!/bin/bash
#
# Build the ipfs-cluster snaps and push them to the store using a docker container.

set -ev

docker run -v $(pwd):$(pwd) -t snapcore/snapcraft sh -c "apt update -qq && cd $(pwd) && for arch in amd64 i386 armhf arm64; do snapcraft snap --target-arch \$arch && snapcraft clean && snapcraft push ipfs-cluster*\$arch.snap --release edge; done"
