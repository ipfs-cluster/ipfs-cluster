#!/bin/sh

set -e
user=ipfs
repo="$IPFS_PATH"
echo "Version 0.4"

if [ `id -u` -eq 0 ]; then
    # ensure folders are writeable
    su-exec "$user" test -w "$repo" || chown -R -- "$user" "$repo"
    su-exec "$user" test -w "$IPFS_CLUSTER_PATH" || chown -R -- "$user" "$IPFS_CLUSTER_PATH"
    # restart script with new privileges
    exec su-exec "$user" "$0" "$@"
fi

# Second invocation with regular user
echo "Second invocation"

if [ `id -u` -eq 0 ]; then
    # ensure directories are writable
    su-exec "$user" test -w "$repo" || chown -R -- "$user" "$repo"
    su-exec "$user" test -w "$IPFS_CLUSTER_PATH" || chown -R -- "$user" "$IPFS_CLUSTER_PATH"
    exec su-exec "$user" "$0" "$@"
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

ipfs-cluster-service --version

if [ -e "$IPFS_CLUSTER_PATH/service.json" ]; then
    echo "Found IPFS cluster configuration at $IPFS_CLUSTER_PATH"
else
    ipfs-cluster-service init
    sed -i 's/127\.0\.0\.1\/tcp\/9094/0.0.0.0\/tcp\/9094/' "$IPFS_CLUSTER_PATH/service.json"
    sed -i 's/127\.0\.0\.1\/tcp\/9095/0.0.0.0\/tcp\/9095/' "$IPFS_CLUSTER_PATH/service.json"
fi
ipfs-cluster-service $@ &
echo "Daemons launched"
exec tail -f /dev/null
