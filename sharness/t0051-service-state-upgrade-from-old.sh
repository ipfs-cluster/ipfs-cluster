#!/bin/bash

test_description="Test service state upgrade v1 -> v2 and v2 -> v2"

. lib/test-lib.sh

test_ipfs_init
cleanup test_clean_ipfs
test_cluster_init
test_confirm_v1State
cleanup test_clean_cluster

# Make a pin and shutdown to force a snapshot. Modify snapshot files to specify
# a snapshot of v1 state pinning "test" (it's easier than taking a new one each
# time with the correct metadata). Upgrade state, check that the correct
# pin cid is in the state
test_expect_success IPFS,CLUSTER,V1STATE,JQ "cluster-service loads v1 state correctly" '
     cid=`docker exec ipfs sh -c "echo test | ipfs add -q"` &&
     cid2=`docker exec ipfs sh -c "echo testing | ipfs add -q"` &&
     ipfs-cluster-ctl pin add "$cid2" &&
     cluster_kill &&
     sleep 15 &&
     SNAP_DIR=`find test-config/ipfs-cluster-data/snapshots/ -maxdepth 1 -mindepth 1 | head -n 1` &&
     cp v1State "$SNAP_DIR/state.bin" &&
     cat "$SNAP_DIR/meta.json" | jq --arg CRC "$V1_CRC" '"'"'.CRC = $CRC'"'"' > tmp.json &&
     cp tmp.json "$SNAP_DIR/meta.json" &&
     ipfs-cluster-service --debug --config "test-config" state upgrade &&
     cluster_start &&
     ipfs-cluster-ctl pin ls "$cid" | grep -q "$cid" &&
     ipfs-cluster-ctl status "$cid" | grep -q -i "PINNED"
'

test_done
