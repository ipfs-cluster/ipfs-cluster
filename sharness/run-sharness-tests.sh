#!/bin/bash

# Run tests
cd "$(dirname "$0")"
statuses=0
for i in t0*.sh;
do
    echo "*** $i ***"
    ./$i
    status=$?
    statuses=$((statuses + $status))
    if [ $status -ne 0 ]; then
       echo "Test $i failed"
    fi
done

# Aggregate Results
echo "Aggregating..."
for f in test-results/*.counts; do
    echo "$f";
done | bash lib/sharness/aggregate-results.sh

# Cleanup results
rm -rf test-results

# Exit with error if any test has failed
if [ $statuses -gt 0 ]; then
    echo $statuses
    exit 1
fi
exit 0
