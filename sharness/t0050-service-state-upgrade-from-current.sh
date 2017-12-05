#!/bin/bash

test_description="Test service state 'upgrade' from current version"

. lib/test-lib.sh

test_ipfs_init
cleanup test_clean_ipfs
test_cluster_init
cleanup test_clean_cluster

test_expect_success IPFS,CLUSTER "cluster-service state upgrade works" '
    cid=`docker exec ipfs sh -c "echo testing | ipfs add -q"` &&
    ipfs-cluster-ctl pin add "$cid" &&
    sleep 5 &&
    cluster_kill &&
    sleep 5 &&
    ipfs-cluster-service --debug --config "test-config" state upgrade
'

# previous test kills the cluster, we need to re-start
# if done inside the test, we lose debugging output
cluster_start

test_expect_success IPFS,CLUSTER "state is preserved after migration" '
    cid=`docker exec ipfs sh -c "echo testing | ipfs add -q"` &&
    ipfs-cluster-ctl pin ls "$cid" | grep -q "$cid" &&
    ipfs-cluster-ctl status "$cid" | grep -q -i "PINNED"
'

test_done
