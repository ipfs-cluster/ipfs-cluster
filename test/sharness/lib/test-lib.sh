# Sharness test framework for ipfs-cluster
#
# MIT Licensed; see the LICENSE file in this repository
#
# We are using sharness (https://github.com/mlafeldt/sharness)
# which was extracted from the Git test framework.

SHARNESS_LIB="lib/sharness/sharness.sh"

. "$SHARNESS_LIB" || {
    echo >&2 "Cannot source: $SHARNESS_LIB"
    echo >&2 "Please check Sharness installation."
    exit 1
}

test_must_be_empty() {
   if test -s "$1"
   then
        echo "'$1' is not empty, it contains:"
        cat "$1"
        return 1
    fi
}

test_should_contain() {
    test "$#" = 2 || error "bug in the test script: not 2 parameters to test_should_contain"
    if ! grep -q "$1" "$2"
    then
        echo "'$2' does not contain '$1', it contains:"
        cat "$2"
        return 1
    fi
}

test_str_contains() {
    find=$1
    shift
    echo "$@" | egrep "\b$find\b" >/dev/null
}

test_fsh() {
    echo "> $@"
    eval $(shellquote "$@")
    echo ""
    false
}

test_kill_daemon() {
    echo "Killing ipfs daemon" 
    ps | grep "ipfs daemon" | grep -v "grep" > daemon_ps.txt
    sed -i '' 's/^ *//' daemon_ps.txt
    awk '{print$1}' daemon_ps.txt > daemon_pid.txt &&
    kill -9 $(< daemon_pid.txt) 
} 

