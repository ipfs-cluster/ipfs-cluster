#!/bin/sh

set -e
user=ipfs
repo="$IPFS_PATH"
cluster_repo="$IPFS_CLUSTER_PATH"
echo "Version 0.4"

if [ `id -u` -eq 0 ]; then
  # ensure folders are writeable
  su-exec "$user" test -w "$repo" || chown -R -- "$user" "$repo"
  su-exec "$user" test -w "$cluster_repo" || chown -R -- "$user" "$cluster_repo" 

  # restart script with new privileges
  exec su-exec "$user" "$0" "$@"
fi

#Second invocation of the script with regular user
echo "Second invocation"

# Test whether the mounted directory is writable for us
if [ ! -w "$repo" 2>/dev/null ]; then
  echo "error: $repo is not writable for user $user (uid=$(id -u $user))"
  ls -l "/data"
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

echo "Confirming experimental container"
ipfs daemon --migrate=true &
sleep 3

# Test whether the mounted directory is writable for us
if [ ! -w "$cluster_repo" 2>/dev/null ]; then
  echo "error: $cluster_repo is not writable for user $user (uid=$(id -u $user))"
  exit 1
fi

ipfs-cluster-service --version

if [ -e "$cluster_repo/service.json" ]; then
    echo "Found IPFS cluster configuration at $IPFS_CLUSTER_PATH"
else
    ipfs-cluster-service init
    sed -i 's/127\.0\.0\.1\/tcp\/9094/0.0.0.0\/tcp\/9094/' "$IPFS_CLUSTER_PATH/service.json"
    sed -i 's/127\.0\.0\.1\/tcp\/9095/0.0.0.0\/tcp\/9095/' "$IPFS_CLUSTER_PATH/service.json"
fi

ipfs-cluster-service $@ &

# Parent process blocks forever. Least ugly solution (still ugly)
echo "Daemons launched"
exec tail -f /dev/null
