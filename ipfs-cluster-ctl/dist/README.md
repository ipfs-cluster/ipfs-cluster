# `ipfs-cluster-ctl`

> IPFS cluster management tool

`ipfs-cluster-ctl` is the client application to manage the cluster nodes and perform actions. `ipfs-cluster-ctl` uses the HTTP API provided by the nodes and it is completely separate from the cluster service.


### Usage

Usage information can be obtained by running:

```
$ ipfs-cluster-ctl --help
```

You can also obtain command-specific help with `ipfs-cluster-ctl help [cmd]`. The (`--host`) can be used to talk to any remote cluster peer (`localhost` is used by default). In summary, it works as follows:


```
$ ipfs-cluster-ctl id                                                       # show cluster peer and ipfs daemon information
$ ipfs-cluster-ctl peers ls                                                 # list cluster peers
$ ipfs-cluster-ctl peers rm <peerid>                                        # remove a cluster peer
$ ipfs-cluster-ctl pin add Qma4Lid2T1F68E3Xa3CpE6vVJDLwxXLD8RfiB9g1Tmqp58   # pins a CID in the cluster
$ ipfs-cluster-ctl pin rm Qma4Lid2T1F68E3Xa3CpE6vVJDLwxXLD8RfiB9g1Tmqp58    # unpins a CID from the clustre
$ ipfs-cluster-ctl pin ls [CID]                                             # list tracked CIDs (shared state)
$ ipfs-cluster-ctl status [CID]                                             # list current status of tracked CIDs (local state)
$ ipfs-cluster-ctl sync Qma4Lid2T1F68E3Xa3CpE6vVJDLwxXLD8RfiB9g1Tmqp58      # re-sync seen status against status reported by the IPFS daemon
$ ipfs-cluster-ctl recover Qma4Lid2T1F68E3Xa3CpE6vVJDLwxXLD8RfiB9g1Tmqp58   # attempt to re-pin/unpin CIDs in error state
```

#### Exit codes

`ipfs-cluster-ctl` will exit with:

* `0`: the request/operation succeeded. The output contains the response data.
* `1`: argument error, network error or any other error which prevented the application to perform a request and obtain a response from the ipfs-cluster API. In such case, the output contains the contents of the error and the http code `0`.
* `2`: ipfs-cluster error. The request was performed correctly but the response is an error (http status 4xx or 5xx). In such case, the output contains the contents of the error and the HTTP code associated to it.

### Debugging

`ipfs-cluster-ctl` takes a `--debug` flag which allows to inspect request paths and raw response bodies.
