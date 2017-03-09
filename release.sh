#!/bin/bash

# Updates the Version variables, commits, and "gx release" the package

version="$1"

if [ -z $version ]; then
   echo "Need a version!"
   exit 1  
fi

sed -i "s/const Version.*$/const Version = \"$version\"/" version.go
sed -i "s/const Version.*$/const Version = \"$version\"/" ipfs-cluster-ctl/main.go
git commit -a -m "Release $version"
git tag v$version
gx release $version 
