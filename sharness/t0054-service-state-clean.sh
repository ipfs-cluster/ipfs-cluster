#!/bin/bash

test_description="Test service state import"

. lib/test-lib.sh

test_ipfs_init
cleanup test_clean_ipfs
test_cluster_init
cleanup test_clean_cluster

test_expect_success IPFS,CLUSTER "state cleanup refreshes state on restart" '
     cid=`docker exec ipfs sh -c "echo test_54 | ipfs add -q"` &&
     ipfs-cluster-ctl pin add "$cid" && sleep 5 &&
     ipfs-cluster-ctl pin ls "$cid" | grep -q "$cid" &&
     ipfs-cluster-ctl status "$cid" | grep -q -i "PINNED" &&
     [ 1 -eq "$(ipfs-cluster-ctl --enc=json status | jq ". | length")" ] &&
     cluster_kill && sleep 5 &&
     ipfs-cluster-service -f --debug --config "test-config" state cleanup &&
     cluster_start && sleep 5 &&
     [ 0 -eq "$(ipfs-cluster-ctl --enc=json status | jq ". | length")" ]
'

test_expect_success IPFS,CLUSTER "export + cleanup + import == noop" '
    cid=`docker exec ipfs sh -c "echo test_54 | ipfs add -q"` &&
    ipfs-cluster-ctl pin add "$cid" && sleep 5 &&		   
    [ 1 -eq "$(ipfs-cluster-ctl --enc=json status | jq ". | length")" ] &&
    cluster_kill && sleep 5 &&
    ipfs-cluster-service --debug --config "test-config" state export -f import.json &&
    ipfs-cluster-service -f --debug --config "test-config" state cleanup &&
    ipfs-cluster-service -f --debug --config "test-config" state import import.json &&
    cluster_start && sleep 5 &&
    ipfs-cluster-ctl pin ls "$cid" | grep -q "$cid" &&
    ipfs-cluster-ctl status "$cid" | grep -q -i "PINNED" &&
    [ 1 -eq "$(ipfs-cluster-ctl --enc=json status | jq ". | length")" ]
'

test_done
