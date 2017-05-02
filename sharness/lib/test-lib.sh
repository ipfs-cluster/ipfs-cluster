# Sharness test framework for ipfs-cluster
#
# We are using sharness (https://github.com/mlafeldt/sharness)
# which was extracted from the Git test framework.

SHARNESS_LIB="lib/sharness/sharness.sh"

. "$SHARNESS_LIB" || {
    echo >&2 "Cannot source: $SHARNESS_LIB"
    echo >&2 "Please check Sharness installation."
    exit 1
}

# Set prereqs 
test_ipfs_init() {
    ipfs help | egrep -q -i "^Usage" &&
    IPFS_TEMP_DIR=`mktemp -d ipfs-XXXXX` && # Store in TEMP_DIR for safer delete
    export IPFS_PATH=$IPFS_TEMP_DIR &&
    ipfs init && 
    eval 'ipfs daemon & export IPFS_D_PID=`echo $!`' && # Esoteric, but gets correct value of $!
    sleep 2 &&
    test_set_prereq IPFS_INIT
}

test_cluster_init() {
    ipfs-cluster-service help | egrep -q -i "^Usage" &&
    CLUSTER_TEMP_DIR=`mktemp -d cluster-XXXXX` &&
    ipfs-cluster-service -f --config $CLUSTER_TEMP_DIR init &&
    CLUSTER_CONFIG_PATH=$CLUSTER_TEMP_DIR"/service.json" &&
    CLUSTER_CONFIG_ID=`jq --raw-output ".id" $CLUSTER_CONFIG_PATH` &&
    CLUSTER_CONFIG_PK=`jq --raw-output ".private_key" $CLUSTER_CONFIG_PATH` &&
    [ $CLUSTER_CONFIG_ID != null ] &&
    [ $CLUSTER_CONFIG_PK != null ] &&
    eval 'ipfs-cluster-service --config $CLUSTER_TEMP_DIR & export CLUSTER_D_PID=`echo $!`' && 
    sleep 2 &&
    test_set_prereq CLUSTER_INIT
}


# Cleanup functions
test_clean_ipfs(){
    kill -1 $IPFS_D_PID &&
    rm -rf $IPFS_TEMP_DIR    # Remove temp_dir not path in case this is called before init
}

test_clean_cluster(){
    kill -1 $CLUSTER_D_PID &&
    rm -rf $CLUSTER_TEMP_DIR 
}
