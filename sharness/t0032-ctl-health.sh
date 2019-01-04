#!/bin/bash

test_description="Test cluster-ctl's information monitoring functionality"

. lib/test-lib.sh

test_ipfs_init
test_cluster_init

test_expect_success IPFS,CLUSTER "health graph succeeds and prints as expected" '
    ipfs-cluster-ctl health graph | grep -q "C0 -> I0"
'

test_expect_success IPFS,CLUSTER "health metrics with metric name must succeed" '
    ipfs-cluster-ctl health metrics ping &&
    ipfs-cluster-ctl health metrics freespace
'

test_expect_success IPFS,CLUSTER "health metrics without metric name fails" '
    test_must_fail ipfs-cluster-ctl health metrics
'

test_expect_success IPFS,CLUSTER "list latest metrics logged by this peer" '
    pid=`docker exec ipfs sh -c "ipfs id | jq .ID"`
    ipfs-cluster-ctl health metrics freespace | grep -q "$pid: [0-9]* | Expire: .*T.*Z"
'

test_clean_ipfs
test_clean_cluster

test_done
