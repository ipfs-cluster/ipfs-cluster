#!/bin/bash

test_description="Test cluster-ctl's add functionality"

. lib/test-lib.sh

test_ipfs_init
test_cluster_init

test_expect_success IPFS,CLUSTER "add files locally and compare with ipfs" '
    dd if=/dev/urandom bs=1M count=20 of=bigfile.bin
    dd if=/dev/urandom bs=1 count=500 of=smallfile.bin
    mkdir -p testFolder/subfolder
    echo "abc" > testFolder/abc.txt
    cp bigfile.bin testFolder/subfolder/bigfile.bin
    cp smallfile.bin testFolder/smallfile.bin

    docker cp bigfile.bin ipfs:/tmp/bigfile.bin
    docker cp smallfile.bin ipfs:/tmp/smallfile.bin
    docker cp testFolder ipfs:/tmp/testFolder

    ipfs-cluster-ctl add --quiet smallfile.bin > cidscluster.txt
    ipfs-cluster-ctl add --quiet -w smallfile.bin >> cidscluster.txt
    ipfs-cluster-ctl add --quiet --raw-leaves -w smallfile.bin >> cidscluster.txt
    ipfs-cluster-ctl add --quiet --raw-leaves smallfile.bin >> cidscluster.txt

    ipfs-cluster-ctl add --quiet bigfile.bin >> cidscluster.txt
    ipfs-cluster-ctl add --quiet --layout trickle bigfile.bin >> cidscluster.txt
    ipfs-cluster-ctl add --quiet -w bigfile.bin >> cidscluster.txt
    ipfs-cluster-ctl add --quiet --raw-leaves -w bigfile.bin >> cidscluster.txt
    ipfs-cluster-ctl add --quiet --raw-leaves bigfile.bin >> cidscluster.txt

    ipfs-cluster-ctl add --quiet -r testFolder >> cidscluster.txt
    ipfs-cluster-ctl add --quiet -r -w testFolder >> cidscluster.txt

    ipfs-cluster-ctl add --quiet --cid-version 1 -r testFolder >> cidscluster.txt
    ipfs-cluster-ctl add --quiet --hash sha3-512 -r testFolder >> cidscluster.txt

    ipfsCmd add --quiet /tmp/smallfile.bin > cidsipfs.txt
    ipfsCmd add --quiet -w /tmp/smallfile.bin >> cidsipfs.txt
    ipfsCmd add --quiet --raw-leaves -w /tmp/smallfile.bin >> cidsipfs.txt
    ipfsCmd add --quiet --raw-leaves  /tmp/smallfile.bin >> cidsipfs.txt

    ipfsCmd add --quiet /tmp/bigfile.bin >> cidsipfs.txt
    ipfsCmd add --quiet --trickle /tmp/bigfile.bin  >> cidsipfs.txt
    ipfsCmd add --quiet -w /tmp/bigfile.bin >> cidsipfs.txt
    ipfsCmd add --quiet --raw-leaves -w /tmp/bigfile.bin >> cidsipfs.txt
    ipfsCmd add --quiet --raw-leaves /tmp/bigfile.bin >> cidsipfs.txt

    ipfsCmd add --quiet -r /tmp/testFolder >> cidsipfs.txt
    ipfsCmd add --quiet -r -w /tmp/testFolder >> cidsipfs.txt

    ipfsCmd add --quiet --cid-version 1 -r /tmp/testFolder >> cidsipfs.txt
    ipfsCmd add --quiet --hash sha3-512 -r /tmp/testFolder >> cidsipfs.txt

    test_cmp cidscluster.txt cidsipfs.txt
'

# Adding a folder with a single file is the same as adding the file
# and wrapping it.
test_expect_success IPFS,CLUSTER "check add folders" '
    mkdir testFolder2
    echo "abc" > testFolder2/abc.txt
    ipfs-cluster-ctl add --quieter -w testFolder2/abc.txt > wrapped.txt
    ipfs-cluster-ctl add --quieter -r testFolder2 > folder.txt
    test_cmp wrapped.txt folder.txt
'

test_expect_success IPFS,CLUSTER "check pin after locally added" '
    mkdir testFolder3
    echo "abc" > testFolder3/abc.txt
    cid=`ipfs-cluster-ctl add -r --quieter testFolder3`
    ipfs-cluster-ctl pin ls | grep -q -i "$cid"
'

test_clean_ipfs
test_clean_cluster

test_done
