#!/bin/bash

test_description="Test service state export"

. lib/test-lib.sh

test_ipfs_init
test_cluster_init


test_expect_success IPFS,CLUSTER "state export fails without snapshots" '
    cluster_kill
    sleep 5
    test_expect_code 1 ipfs-cluster-service --debug --config "test-config" state export
'

test_clean_cluster
test_cluster_init

test_expect_success IPFS,CLUSTER,JQ "state export saves the correct state to expected file" '
    cid=`docker exec ipfs sh -c "echo test_52 | ipfs add -q"` &&
    ipfs-cluster-ctl pin add "$cid" &&
    sleep 5 &&
    cluster_kill && sleep 5 &&
    ipfs-cluster-service --debug --config "test-config" state export -f export.json &&
    [ -f export.json ] &&
    jq ".[].cid" export.json | grep -q "$cid"
'

test_clean_ipfs
test_clean_cluster

test_done
