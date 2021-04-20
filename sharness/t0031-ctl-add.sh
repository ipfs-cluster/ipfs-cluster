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

    ipfs-cluster-ctl add  smallfile.bin > cidscluster.txt
    ipfs-cluster-ctl add  -w smallfile.bin >> cidscluster.txt

    ipfs-cluster-ctl add  --raw-leaves -w smallfile.bin >> cidscluster.txt
    ipfs-cluster-ctl add  --raw-leaves smallfile.bin >> cidscluster.txt

    ipfs-cluster-ctl add  bigfile.bin >> cidscluster.txt
    ipfs-cluster-ctl add  --layout trickle bigfile.bin >> cidscluster.txt
    ipfs-cluster-ctl add  -w bigfile.bin >> cidscluster.txt
    ipfs-cluster-ctl add  --raw-leaves -w bigfile.bin >> cidscluster.txt
    ipfs-cluster-ctl add  --raw-leaves bigfile.bin >> cidscluster.txt

    ipfs-cluster-ctl add -r testFolder >> cidscluster.txt
    ipfs-cluster-ctl add -r -w testFolder >> cidscluster.txt

    ipfs-cluster-ctl add --cid-version 1 -r testFolder >> cidscluster.txt
    ipfs-cluster-ctl add --hash sha3-512 -r testFolder >> cidscluster.txt

    ipfsCmd add  /tmp/smallfile.bin > cidsipfs.txt
    ipfsCmd add  -w /tmp/smallfile.bin >> cidsipfs.txt
    
    ipfsCmd add  --raw-leaves -w /tmp/smallfile.bin >> cidsipfs.txt
    ipfsCmd add  --raw-leaves  /tmp/smallfile.bin >> cidsipfs.txt

    ipfsCmd add  /tmp/bigfile.bin >> cidsipfs.txt
    ipfsCmd add  --trickle /tmp/bigfile.bin  >> cidsipfs.txt
    ipfsCmd add  -w /tmp/bigfile.bin >> cidsipfs.txt
    ipfsCmd add  --raw-leaves -w /tmp/bigfile.bin >> cidsipfs.txt
    ipfsCmd add  --raw-leaves /tmp/bigfile.bin >> cidsipfs.txt

    ipfsCmd add  -r /tmp/testFolder >> cidsipfs.txt
    ipfsCmd add  -r -w /tmp/testFolder >> cidsipfs.txt

    ipfsCmd add  --cid-version 1 -r /tmp/testFolder >> cidsipfs.txt
    ipfsCmd add  --hash sha3-512 -r /tmp/testFolder >> cidsipfs.txt

    test_cmp cidscluster.txt cidsipfs.txt
'

test_expect_success IPFS,CLUSTER "add CAR file" '
    mkdir testFolderCar
    echo "abc" > testFolderCar/abc.txt
    docker cp testFolderCar ipfs:/tmp/testFolderCar

    ipfsCmd add -Q -w -r /tmp/testFolderCar >> caripfs.txt
    ipfsCmd dag export `cat caripfs.txt` > test.car
    docker cp ipfs:/tmp/test.car test.car
    ipfs-cluster-ctl add --format car -Q test.car >> carcluster.txt
    test_cmp carcluster.txt caripfs.txt
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

test_expect_success IPFS,CLUSTER "add with metadata" '
    echo "test1" > test1.txt
    cid1=`ipfs-cluster-ctl add --quieter --metadata kind=text test1.txt`
    echo "test2" > test2.txt
    cid2=`ipfs-cluster-ctl add --quieter test2.txt`
    ipfs-cluster-ctl pin ls "$cid1" | grep -q "Metadata: yes" &&
    ipfs-cluster-ctl --enc=json pin ls "$cid1" | jq .metadata | grep -q "\"kind\": \"text\"" &&
    ipfs-cluster-ctl pin ls "$cid2" | grep -q "Metadata: no"
'

test_clean_ipfs
test_clean_cluster

test_done
