#!/bin/bash

test_description="Test cluster-ctl's pinning and unpinning functionality"

. lib/test-lib.sh

test_ipfs_init
test_cluster_init

test_expect_success IPFS,CLUSTER "pin data to cluster with ctl" '
    cid=`docker exec ipfs sh -c "echo test | ipfs add -q"`
    ipfs-cluster-ctl pin add "$cid" &&
    ipfs-cluster-ctl pin ls "$cid" | grep -q "$cid" &&
    ipfs-cluster-ctl status "$cid" | grep -q -i "PINNED"
'

test_expect_success IPFS,CLUSTER "unpin data from cluster with ctl" '
    cid=`ipfs-cluster-ctl --enc=json pin ls | jq -r ".[] | .cid" | head -1`
    ipfs-cluster-ctl pin rm "$cid" &&
    !(ipfs-cluster-ctl pin ls "$cid" | grep -q "$cid") &&
    ipfs-cluster-ctl status "$cid" | grep -q -i "UNPINNED"
'

test_expect_success IPFS,CLUSTER "wait for data to pin to cluster with ctl" '
    cid=`docker exec ipfs sh -c "dd if=/dev/urandom bs=1024 count=2048 | ipfs add -q"`
    ipfs-cluster-ctl pin add --wait "$cid" | grep -q -i "PINNED" &&
    ipfs-cluster-ctl pin ls "$cid" | grep -q "$cid" &&
    ipfs-cluster-ctl status "$cid" | grep -q -i "PINNED"
'

test_expect_success IPFS,CLUSTER "wait for data to unpin from cluster with ctl" '
    cid=`ipfs-cluster-ctl --enc=json pin ls | jq -r ".[] | .cid" | head -1`
    ipfs-cluster-ctl pin rm --wait "$cid" | grep -q -i "UNPINNED" &&
    !(ipfs-cluster-ctl pin ls "$cid" | grep -q "$cid") &&
    ipfs-cluster-ctl status "$cid" | grep -q -i "UNPINNED"
'

test_expect_success IPFS,CLUSTER "wait for data to pin to cluster with ctl with timeout" '
    cid=`docker exec ipfs sh -c "dd if=/dev/urandom bs=1024 count=2048 | ipfs add -q"`
    ipfs-cluster-ctl pin add --wait --wait-timeout 2s "$cid" | grep -q -i "PINNED" &&
    ipfs-cluster-ctl pin ls "$cid" | grep -q "$cid" &&
    ipfs-cluster-ctl status "$cid" | grep -q -i "PINNED"
'

test_expect_success IPFS,CLUSTER "wait for data to unpin from cluster with ctl with timeout" '
    cid=`ipfs-cluster-ctl --enc=json pin ls | jq -r ".[] | .cid" | head -1`
    ipfs-cluster-ctl pin rm --wait --wait-timeout 2s "$cid" | grep -q -i "UNPINNED" &&
    !(ipfs-cluster-ctl pin ls "$cid" | grep -q "$cid") &&
    ipfs-cluster-ctl status "$cid" | grep -q -i "UNPINNED"
'

cid=(`docker exec ipfs sh -c "mkdir -p test1/test2 && touch test1/test2/test3.txt && ipfs add -qr test1"`)

test_expect_success IPFS,CLUSTER "pin data to cluster with ctl using ipfs paths" '
    ipfs-cluster-ctl pin add "/ipfs/${cid[2]}/test2/test3.txt" &&
    ipfs-cluster-ctl pin ls "${cid[0]}" | grep -q "${cid[0]}" &&
    ipfs-cluster-ctl status "${cid[0]}" | grep -q -i "PINNED"
'

test_expect_success IPFS,CLUSTER "unpin data to cluster with ctl using ipfs paths" '
    removed=(`ipfs-cluster-ctl pin rm "/ipfs/${cid[2]}/test2/test3.txt"`) &&
    echo "${removed[0]}" | grep -q "${cid[0]}" &&
    !(ipfs-cluster-ctl pin ls "${cid[0]}" | grep -q "${cid[0]}") &&
    ipfs-cluster-ctl status "${cid[0]}" | grep -q -i "UNPINNED"
'

test_expect_success IPFS,CLUSTER "pin data to cluster with ctl using ipns paths" '
    name=`docker exec ipfs sh -c "ipfs name publish -Q ${cid[0]}"`
    ipfs-cluster-ctl pin add "/ipns/$name" &&
    ipfs-cluster-ctl pin ls "${cid[0]}" | grep -q "${cid[0]}" &&
    ipfs-cluster-ctl status "${cid[0]}" | grep -q -i "PINNED"
'

test_expect_success IPFS,CLUSTER "unpin data to cluster with ctl using ipns paths" '
    removed=(`ipfs-cluster-ctl pin rm "/ipns/$name"`) &&
    echo "${removed[0]}" | grep -q "${cid[0]}" &&
    !(ipfs-cluster-ctl pin ls "${cid[0]}" | grep -q "${cid[0]}") &&
    ipfs-cluster-ctl status "${cid[0]}" | grep -q -i "UNPINNED"
'

test_expect_success IPFS,CLUSTER "pin data to cluster with user allocations" '
    pid=`ipfs-cluster-ctl --enc=json id | jq -r ".id"`
    ipfs-cluster-ctl pin add --allocations ${pid} -r 1 "${cid[0]}"
    ipfs-cluster-ctl pin ls "${cid[0]}" | grep -q "${cid[0]}" &&
    ipfs-cluster-ctl status "${cid[0]}" | grep -q -i "PINNED"
    allocations=`ipfs-cluster-ctl --enc=json pin ls | jq .[0].allocations[]`
    echo $allocations | wc -w | grep -q 1 &&
    echo $allocations | grep -q ${pid}
'

test_clean_ipfs
test_clean_cluster

test_done
