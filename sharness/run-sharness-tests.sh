#!/bin/sh

# Run tests
cd "$(dirname "$0")"
exit_codes=()
for i in t*.sh;
do
    echo "*** $i ***"
    ./$i
    exit_codes+=($?)
done

# Aggregate Results
echo "Aggregating..."
for f in test-results/*.counts; do
    echo "$f";
done | bash lib/sharness/aggregate-results.sh

# If any test fails error on exit
for exit_code in "${exit_codes[@]}"; do
    if [[ $exit_code != 0 ]]; then
        exit 1
    fi
done
