#!/bin/bash

test_description="Test service state import"

. lib/test-lib.sh

test_ipfs_init
test_cluster_init
test_confirm_importState

# Kill cluster daemon but keep data folder
cluster_kill


# WARNING: Updating the added content needs updating the importState file.

test_expect_success IPFS,CLUSTER "state import fails on incorrect format (crdt)" '
    sleep 5 &&
    echo "not exactly json" > badImportFile &&
    test_expect_code 1 ipfs-cluster-service --config "test-config" state import -f badImportFile
'

test_expect_success IPFS,CLUSTER,IMPORTSTATE "state import succeeds on correct format (crdt)" '
    sleep 5
    cid=`docker exec ipfs sh -c "echo test_53 | ipfs add -q"` &&
    ipfs-cluster-service --config "test-config" state import -f importState &&
    cluster_start &&
    sleep 5 &&
    ipfs-cluster-ctl pin ls "$cid" | grep -q "$cid" &&
    ipfs-cluster-ctl status "$cid" | grep -q -i "PINNED"
'

# Kill cluster daemon but keep data folder
cluster_kill
sleep 5

test_expect_success IPFS,CLUSTER "state import fails on incorrect format (raft)" '
    ipfs-cluster-service --config "test-config" init --force --consensus raft &&
    echo "not exactly json" > badImportFile &&
    test_expect_code 1 ipfs-cluster-service --config "test-config" state import -f badImportFile
'

test_expect_success IPFS,CLUSTER,IMPORTSTATE "state import succeeds on correct format (raft)" '
    sleep 5
    cid=`docker exec ipfs sh -c "echo test_53 | ipfs add -q"` &&
    ipfs-cluster-service --config "test-config" state import -f importState &&
    cluster_start &&
    sleep 5 &&
    ipfs-cluster-ctl pin ls "$cid" | grep -q "$cid" &&
    ipfs-cluster-ctl status "$cid" | grep -q -i "PINNED"
'

test_clean_ipfs
test_clean_cluster

test_done
