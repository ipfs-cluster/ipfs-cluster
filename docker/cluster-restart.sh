#! /bin/bash

# Restart the cluster process
sleep 2
while true; do
  export CLUSTER_SECRET=""
  pgrep ipfs-cluster-service || echo "CLUSTER RESTARTED"; ipfs-cluster-service --debug &
  sleep 10
done
