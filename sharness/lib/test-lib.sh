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
