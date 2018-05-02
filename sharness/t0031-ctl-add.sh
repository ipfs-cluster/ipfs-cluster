#!/bin/bash

test_description="Test cluster-ctl's add functionality"

. lib/test-lib.sh

test_ipfs_init
test_cluster_init

test_expect_success IPFS,CLUSTER "add small file to cluster with ctl" '
    cid=`ipfs-cluster-ctl add --only-hashes ../test_data/small_file | tail -1` &&
    ipfs-cluster-ctl pin ls | grep -q "$cid" &&
    ipfs-cluster-ctl pin rm $cid &&
    [[ -z "$(ipfs-cluster-ctl pin ls)" ]] 
'

test_expect_success IPFS,CLUSTER "add sharded small file to cluster" '
    cid=`ipfs-cluster-ctl add --only-hashes --shard ../test_data/small_file | tail -1` &&
    [[ -z "$(ipfs-cluster-ctl pin ls)" ]] &&
    ipfs-cluster-ctl pin ls -a | grep -q "$cid" &&
    [[ $(ipfs-cluster-ctl pin ls -a | wc -l) -eq "3" ]]  &&         
    ipfs-cluster-ctl pin rm $cid &&
    [[ -z "$(ipfs-cluster-ctl pin ls -a)" ]] 
'

test_expect_success IPFS,CLUSTER "add same file sharded and unsharded" '
    cid=`ipfs-cluster-ctl add --only-hashes --shard ../test_data/small_file | tail -1` &&
    test_expect_code 2 ipfs-cluster-ctl add ../test_data/small_file &&
    ipfs-cluster-ctl pin rm $cid &&
    ipfs-cluster-ctl add ../test_data/small_file
'
								     

test_clean_ipfs
test_clean_cluster

test_done
