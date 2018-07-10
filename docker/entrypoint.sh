#!/bin/sh

set -e
user=ipfs

if [ -n "$DOCKER_DEBUG" ]; then
   set -x
fi

if [ `id -u` -eq 0 ]; then
    echo "Changing user to $user"
    # ensure directories are writable
    su-exec "$user" test -w "${IPFS_CLUSTER_PATH}" || chown -R -- "$user" "${IPFS_CLUSTER_PATH}"
    exec su-exec "$user" "$0" $@
fi

# Only ipfs user can get here
ipfs-cluster-service --version

if [ -e "${IPFS_CLUSTER_PATH}/service.json" ]; then
    echo "Found IPFS cluster configuration at ${IPFS_CLUSTER_PATH}"
else
    echo "This container only runs ipfs-cluster-service. ipfs needs to be run separately!"
    echo "Initializing default configuration"
    ipfs-cluster-service init
    if [ -n "$IPFS_API" ]; then
        echo "Using \$IPFS_API as value for ipfs_connector.ipfshttp.node_multiaddress in configuration"
        sed -i "s;/ip4/127\.0\.0\.1/tcp/5001;$IPFS_API;" "${IPFS_CLUSTER_PATH}/service.json"
    fi

    echo "Warning: By default, the API and Proxy endpoints will listen on 0.0.0.0!"
    sed -i 's;127\.0\.0\.1/tcp/9094;0.0.0.0/tcp/9094;' "${IPFS_CLUSTER_PATH}/service.json"
    sed -i 's;127\.0\.0\.1/tcp/9095;0.0.0.0/tcp/9095;' "${IPFS_CLUSTER_PATH}/service.json"
fi

exec ipfs-cluster-service $@
