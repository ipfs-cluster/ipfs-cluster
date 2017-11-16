#!/bin/bash
#
# Build the ipfs-cluster snaps and push them to the store using a docker container.

set -ev

docker run -v $(pwd):$(pwd) -t snapcore/snapcraft sh -c "apt update -qq && cd $(pwd) && ./snap/snap-multiarch.sh"
