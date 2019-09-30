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

test_expect_success IPFS,CLUSTER "health metrics without metric name doesn't fail" '
    ipfs-cluster-ctl health metrics
'

test_expect_success IPFS,CLUSTER "list latest metrics logged by this peer" '
    pid=`ipfs-cluster-ctl --enc=json id | jq -r ".id"`
    ipfs-cluster-ctl health metrics freespace | grep -q -E "(^$pid \| freespace: [0-9]+ (G|M|K)B \| Expires in: [0-9]+ seconds from now)"
'

test_clean_ipfs
test_clean_cluster

test_done
