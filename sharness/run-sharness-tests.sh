#!/bin/sh

# Run tests 
cd "$(dirname "$0")"
for i in `ls t*.sh | sort`;
do
    echo "*** $i ***"
    ./$i 
done

# Aggregate Results
echo "Aggregating..."
for f in test-results/*.counts; do
    echo "$f";
done | bash lib/sharness/aggregate-results.sh
