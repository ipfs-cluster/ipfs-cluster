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

if [ -e "$IPFS_CLUSTER_PATH/service.json" ]; then
    echo "Found IPFS cluster configuration at $IPFS_CLUSTER_PATH"
else
    export CLUSTER_SECRET=""
    ipfs-cluster-service init --consensus "${IPFS_CLUSTER_CONSENSUS}"
fi

ipfs-cluster-service --debug $@ &
# Testing scripts that spawn background processes are spawned and stopped here
/usr/local/bin/random-stopper.sh &
kill -STOP $!
echo $! > /data/ipfs-cluster/random-stopper-pid
/usr/local/bin/random-killer.sh &
kill -STOP $!
echo $! > /data/ipfs-cluster/random-killer-pid
/usr/local/bin/cluster-restart.sh &
kill -STOP $!
echo $! > /data/ipfs-cluster/cluster-restart-pid

echo "Daemons launched"
exec tail -f /dev/null
