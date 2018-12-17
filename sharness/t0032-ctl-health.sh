#!/bin/bash

test_description="Test cluster-ctl's information monitoring functionality"

. lib/test-lib.sh

test_ipfs_init
test_cluster_init

test_expect_success IPFS,CLUSTER "list latest metrics logged by this peer" '
    pid=`docker exec ipfs sh -c "ipfs id | jq .ID"`
    ipfs-cluster-ctl health metrics freespace | grep -q "$pid: [0-9]* | Expire: .*T.*Z"
'

test_clean_ipfs
test_clean_cluster

test_done
