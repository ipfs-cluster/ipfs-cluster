#! /bin/bash

# Restart the cluster process
sleep 4
while true; do
  export CLUSTER_SECRET=""
  pgrep ipfs-cluster-service || echo "CLUSTER RESTARTING"; ipfs-cluster-service --debug &
  sleep 10
done
