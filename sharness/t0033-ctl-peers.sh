#!/bin/bash

test_description="Test cluster-ctl's peers command"

. lib/test-lib.sh

test_ipfs_init
test_cluster_init

test_expect_success IPFS,CLUSTER "peers ls gives same results always (crdt)" '
    ipfs-cluster-ctl peers ls > peers1-crdt.txt
    ipfs-cluster-ctl peers ls > peers2-crdt.txt
    test_cmp peers1-crdt.txt peers2-crdt.txt
'

cluster_kill
sleep 5
test_cluster_init "" raft

test_expect_success IPFS,CLUSTER "peers ls gives same results always (raft)" '
    ipfs-cluster-ctl peers ls > peers1-raft.txt
    ipfs-cluster-ctl peers ls > peers2-raft.txt
    test_cmp peers1-raft.txt peers2-raft.txt
'

test_clean_ipfs
test_clean_cluster

test_done
