#!/bin/sh

test_description="Test ctl's status reporting functionality.  Test errors on incomplete commands"

. lib/test-lib.sh

test_ipfs_init
test_cluster_init

test_expect_success IPFS,CLUSTER,JQ "cluster-ctl can read id" '
    test_cluster_config &&
    ipfs-cluster-ctl id | egrep -q -i "$CLUSTER_CONFIG_ID"
'

test_expect_success IPFS,CLUSTER "cluster-ctl list 0 peers" '
    export PEER_OUT=`ipfs-cluster-ctl peers ls` &&
    sorted_peer_out=$(printf "%s\n" $PEER_OUT | sort -u) &&
    export SELF_OUT=`ipfs-cluster-ctl id` &&
    sorted_self_out=$(printf "%s\n" $SELF_OUT | sort -u) &&
    [ "$sorted_peer_out" = "$sorted_self_out" ]
'

test_expect_success IPFS,CLUSTER "cluster-ctl add need peer id" '
    test_must_fail ipfs-cluster-ctl peers add
'

test_expect_success IPFS,CLUSTER "cluster-ctl add invalid peer id" '
    test_must_fail ipfs-cluster-ctl peers add XXXinvalid-peerXXX
'

test_expect_success IPFS,CLUSTER "cluster-ctl rm needs peer id" '
    test_must_fail ipfs-cluster-ctl peers rm
'

test_expect_success IPFS,CLUSTER "cluster-ctl rm invalid peer id" '
    test_must_fail ipfs-cluster-ctl peers rm XXXinvalid-peerXXX
'

test_expect_success IPFS,CLUSTER "empty cluster-ctl status succeeds" '
    ipfs-cluster-ctl status
'

test_expect_success IPFS,CLUSTER "invalid CID status" '
    test_must_fail ipfs-cluster-ctl status XXXinvalid-CIDXXX
'

test_expect_success IPFS,CLUSTER "empty cluster-ctl sync succeeds" '
    ipfs-cluster-ctl sync
'

test_expect_success IPFS,CLUSTER "empty cluster_ctl recover needs CID" '
    test_must_fail ipfs-cluster-ctl recover
'

test_expect_success IPFS,CLUSTER "pin ls succeeds" '
    ipfs-cluster-ctl pin ls
'

test_expect_success IPFS,CLUSTER "pin ls on invalid CID succeeds" '
    ipfs-cluster-ctl pin ls XXXinvalid-CIDXXX
'

cleanup test_clean_cluster
cleanup test_clean_ipfs
test_done
