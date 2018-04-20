#!/bin/bash

test_description="Test cluster-ctl's add functionality"

. lib/test-lib.sh

test_ipfs_init
test_cluster_init

test_expect_success IPFS,CLUSTER "add small file to cluster with ctl" '
    output=`ipfs-cluster-ctl add ../test_data/small_file | tail -1` &&
    cid=${output:7:47} &&
    ipfs-cluster-ctl pin ls | grep -q "$cid"
'

test_expect_success IPFS,CLUSTER "add small file with sharding" '
    echo "complete me"
'
# add, make sure root is in ls -a
# root not in ls
# root is in metapin
# follow clusterdag make sure it points to shard pin
# follow shard pin make sure it points to the root hash and has the correct size

test_expect_success IPFS,CLUSTER "add large file with sharding" '
    echo "complete me"
'

test_clean_ipfs
test_clean_cluster

test_done
