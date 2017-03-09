#!/bin/sh

user=$(whoami)
repo="$IPFS_PATH"

# Test whether the mounted directory is writable for us
if [ ! -w "$repo" 2>/dev/null ]; then
  echo "error: $repo is not writable for user $user (uid=$(id -u $user))"
  exit 1
fi

ipfs version

if [ -e "$repo/config" ]; then
  echo "Found IPFS fs-repo at $repo"
else
  ipfs init
  ipfs config Addresses.API /ip4/0.0.0.0/tcp/5001
  ipfs config Addresses.Gateway /ip4/0.0.0.0/tcp/8080
fi

ipfs daemon --migrate=true &
sleep 3

# Test whether the mounted directory is writable for us
if [ ! -w "$IPFS_CLUSTER_PATH" 2>/dev/null ]; then
  echo "error: $IPFS_CLUSTER_PATH is not writable for user $user (uid=$(id -u $user))"
  exit 1
fi

ipfs-cluster-service --version

if [ -e "$IPFS_CLUSTER_PATH/service.json" ]; then
    echo "Found IPFS cluster configuration at $IPFS_CLUSTER_PATH"
else
    ipfs-cluster-service init
    sed -i 's/127\.0\.0\.1\/tcp\/9094/0.0.0.0\/tcp\/9094/' "$IPFS_CLUSTER_PATH/service.json"
    sed -i 's/127\.0\.0\.1\/tcp\/9095/0.0.0.0\/tcp\/9095/' "$IPFS_CLUSTER_PATH/service.json"
fi

exec ipfs-cluster-service $@
