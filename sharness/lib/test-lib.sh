# Sharness test framework for ipfs-cluster
#
# We are using sharness (https://github.com/mlafeldt/sharness)
# which was extracted from the Git test framework.

SHARNESS_LIB="lib/sharness/sharness.sh"

# Daemons output will be redirected to...
IPFS_OUTPUT="/dev/null" # change for debugging
# IPFS_OUTPUT="/dev/stderr" # change for debugging

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
            echo "Error running go-ipfs in docker."
            exit 1
        fi
        sleep 6
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
    which ipfs-cluster-service >/dev/null 2>&1
    if [ $? -ne 0 ]; then
        echo "ipfs-cluster-service not found"
        exit 1
    fi
    which ipfs-cluster-ctl >/dev/null 2>&1
    if [ $? -ne 0 ]; then
        echo "ipfs-cluster-ctl not found"
        exit 1
    fi
    export CLUSTER_TEMP_DIR=`mktemp -d cluster-XXXXX`
    ipfs-cluster-service -f --config "$CLUSTER_TEMP_DIR" init --gen-secret >"$IPFS_OUTPUT" 2>&1
    if [ $? -ne 0 ]; then
        echo "error initializing ipfs cluster"
        exit 1
    fi
    ipfs-cluster-service --config "$CLUSTER_TEMP_DIR" >"$IPFS_OUTPUT" 2>&1 &
    export CLUSTER_D_PID=$!
    sleep 5
    test_set_prereq CLUSTER
}

test_cluster_config() {
    export CLUSTER_CONFIG_PATH="${CLUSTER_TEMP_DIR}/service.json"
    export CLUSTER_CONFIG_ID=`jq --raw-output ".id" $CLUSTER_CONFIG_PATH`
    export CLUSTER_CONFIG_PK=`jq --raw-output ".private_key" $CLUSTER_CONFIG_PATH`
    [ "$CLUSTER_CONFIG_ID" != "null" ] && [ "$CLUSTER_CONFIG_PK" != "null" ]
}

cluster_id() {
    echo "$CLUSTER_CONFIG_ID"
}

# Cleanup functions
test_clean_ipfs(){
    docker kill ipfs
    docker rm ipfs
    sleep 1
}

test_clean_cluster(){
    kill -1 "$CLUSTER_D_PID"
    rm -rf "$CLUSTER_TEMP_DIR"
}
