#!/bin/sh

set -e
if [ -n "$DOCKER_DEBUG" ]; then
   set -x
fi
user=ipfs

if [ `id -u` -eq 0 ]; then
    echo "Changing user to $user"
    # ensure directories are writable
    su-exec "$user" test -w "${IPFS_PATH}" || chown -R -- "$user" "${IPFS_PATH}"
    su-exec "$user" test -w "${IPFS_CLUSTER_PATH}" || chown -R -- "$user" "${IPFS_CLUSTER_PATH}"
    exec su-exec "$user" "$0" $@
fi

# Second invocation with regular user
ipfs version

if [ -e "${IPFS_PATH}/config" ]; then
  echo "Found IPFS fs-repo at ${IPFS_PATH}"
else
  ipfs init
  ipfs config Addresses.API /ip4/0.0.0.0/tcp/5001
  ipfs config Addresses.Gateway /ip4/0.0.0.0/tcp/8080
fi

ipfs daemon --migrate=true &
sleep 3

ipfs-cluster-service --version

if [ -e "${IPFS_CLUSTER_PATH}/service.json" ]; then
    echo "Found IPFS cluster configuration at ${IPFS_CLUSTER_PATH}"
else
    ipfs-cluster-service init
    sed -i 's/127\.0\.0\.1\/tcp\/9094/0.0.0.0\/tcp\/9094/' "${IPFS_CLUSTER_PATH}/service.json"
    sed -i 's/127\.0\.0\.1\/tcp\/9095/0.0.0.0\/tcp\/9095/' "${IPFS_CLUSTER_PATH}/service.json"
fi

exec ipfs-cluster-service $@
