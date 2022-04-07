#!/bin/bash

test_description="Test service state export"

. lib/test-lib.sh

test_ipfs_init
test_cluster_init "" crdt

test_expect_success IPFS,CLUSTER,JQ "state export saves the correct state to expected file (crdt)" '
    cid=`docker exec ipfs sh -c "echo test_52-1 | ipfs add -q"` &&
    ipfs-cluster-ctl pin add "$cid" &&
    sleep 5 &&
    cluster_kill && sleep 5 &&
    ipfs-cluster-service --debug --config "test-config" state export -f export.json &&
    [ -f export.json ] &&
    jq -r ".cid" export.json | grep -q "$cid"
'

cluster_kill
sleep 5
test_cluster_init "" raft

test_expect_success IPFS,CLUSTER,JQ "state export saves the correct state to expected file (raft)" '
    cid=`docker exec ipfs sh -c "echo test_52-2 | ipfs add -q"` &&
    ipfs-cluster-ctl pin add "$cid" &&
    sleep 5 &&
    cluster_kill && sleep 5 &&
    ipfs-cluster-service --debug --config "test-config" state export -f export.json &&
    [ -f export.json ] &&
    jq -r ".cid" export.json | grep -q "$cid"
'

test_clean_ipfs
test_clean_cluster

test_done
