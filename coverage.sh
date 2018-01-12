#!/bin/bash

echo "mode: count" > fullcov.out
dirs=$(find ./* -maxdepth 10 -type d )
dirs=". $dirs"
for dir in $dirs;
do
        if ls "$dir"/*.go &> /dev/null;
        then
            go test -timeout 20m -v -coverprofile=profile.out -covermode=count -tags silent "$dir"
            if [ $? -ne 0 ];
            then
                exit 1
            fi
            if [ -f profile.out ]
            then
                cat profile.out | grep -v "^mode: count" >> fullcov.out
            fi
        fi
done

if [ -n $COVERALLS_TOKEN ];
then
    $HOME/gopath/bin/goveralls -coverprofile=fullcov.out -service=travis-ci -repotoken $COVERALLS_TOKEN
fi
rm -rf ./profile.out
rm -rf ./fullcov.out
