#!/bin/bash

# Sharness test framework for ipfs-cluster
#
# We are using sharness (https://github.com/mlafeldt/sharness)
# which was extracted from the Git test framework.

SHARNESS_LIB="lib/sharness/sharness.sh"

# Daemons output will be redirected to...
IPFS_OUTPUT="/dev/null"
#IPFS_OUTPUT="/dev/stderr" # uncomment for debugging

. "$SHARNESS_LIB" || {
    echo >&2 "Cannot source: $SHARNESS_LIB"
    echo >&2 "Please check Sharness installation."
    exit 1
}

which jq >/dev/null 2>&1
if [ $? -eq 0 ]; then
    test_set_prereq JQ
fi

# Set prereqs
test_ipfs_init() {
    which docker >/dev/null 2>&1
    if [ $? -ne 0 ]; then
        echo "Docker not found"
        exit 1
    fi
    if docker ps --format '{{.Names}}' | egrep -q '^ipfs$'; then
        echo "ipfs container already running"
    else
        docker run --name ipfs -d -p 127.0.0.1:5001:5001 ipfs/go-ipfs > /dev/null 2>&1
        if [ $? -ne 0 ]; then
            echo "IPFS init FAIL: Error running go-ipfs in docker."
            exit 1
        fi
        while ! curl -s "localhost:5001/api/v0/version" > /dev/null; do
            sleep 0.2
        done
        sleep 2
    fi
    test_set_prereq IPFS
}

test_ipfs_running() {
    if curl -s "localhost:5001/api/v0/version" > /dev/null; then
        test_set_prereq IPFS
    else
        echo "IPFS is not running"
        exit 1
    fi
}

test_cluster_init() {
    custom_config_files="$1"

    which ipfs-cluster-service >/dev/null 2>&1
    if [ $? -ne 0 ]; then
        echo "cluster init FAIL: ipfs-cluster-service not found"
        exit 1
    fi
    which ipfs-cluster-ctl >/dev/null 2>&1
    if [ $? -ne 0 ]; then
        echo "cluster init FAIL: ipfs-cluster-ctl not found"
        exit 1
    fi
    ipfs-cluster-service -f --config "test-config" init >"$IPFS_OUTPUT" 2>&1
    if [ $? -ne 0 ]; then
        echo "cluster init FAIL: error on ipfs cluster init"
        exit 1
    fi
    rm -rf "test-config/ipfs-cluster-data"
    if [ -n "$custom_config_files" ]; then
        cp -f ${custom_config_files}/* "test-config"
    fi
    cluster_start
}

test_cluster_config() {
    export CLUSTER_CONFIG_PATH="test-config/service.json"
    export CLUSTER_CONFIG_ID=`jq --raw-output ".cluster.id" $CLUSTER_CONFIG_PATH`
    export CLUSTER_CONFIG_PK=`jq --raw-output ".cluster.private_key" $CLUSTER_CONFIG_PATH`
    [ "$CLUSTER_CONFIG_ID" != "null" ] && [ "$CLUSTER_CONFIG_PK" != "null" ]
}

cluster_id() {
    jq --raw-output ".cluster.id" test-config/service.json
}

test_confirm_v1State() {
    V1_SNAP_PATH="../test_data/v1State"
    V1_CRC_PATH="../test_data/v1Crc"
    if [ -f $V1_SNAP_PATH ] && [ -f $V1_CRC_PATH ]; then
	export V1_CRC=$(cat ../test_data/v1Crc)
	cp $V1_SNAP_PATH v1State
	test_set_prereq V1STATE
    fi
}

test_confirm_importState() {
    IMP_STATE_PATH="../test_data/importState"
    if [ -f $IMP_STATE_PATH ]; then
	cp $IMP_STATE_PATH importState
	test_set_prereq IMPORTSTATE
    fi
}

cluster_kill(){
    kill -1 "$CLUSTER_D_PID" &>/dev/null
    while pgrep ipfs-cluster-service >/dev/null; do
        sleep 0.2
    done
}

cluster_start(){
    ipfs-cluster-service --config "test-config" daemon >"$IPFS_OUTPUT" 2>&1 &
    export CLUSTER_D_PID=$!
    while ! curl -s 'localhost:9095/api/v0/version' >/dev/null; do
        sleep 0.2
    done
    sleep 5 # wait for leader election
    test_set_prereq CLUSTER
}


# Cleanup functions
test_clean_ipfs(){
    docker kill ipfs >/dev/null
    docker rm ipfs >/dev/null
    sleep 1
}

test_clean_cluster(){
    cluster_kill
    rm -rf 'test-config'
}
