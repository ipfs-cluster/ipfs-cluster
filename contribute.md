# Contribute

This document gathers a few contributing guidelines for ipfs-cluster. We attempt to go to the point and invite the readers eager for more details
to make themselves familiar with:

* The [go-ipfs contributing guidelines](https://github.com/ipfs/go-ipfs/blob/master/contribute.md) and builds upon:
* The [IPFS Community Code of Conduct](https://github.com/ipfs/community/blob/master/code-of-conduct.md)
* The [IFPS community contributing notes](https://github.com/ipfs/community/blob/master/contributing.md)
* The [Go contribution guidelines](https://github.com/ipfs/community/blob/master/go-contribution-guidelines.md)

## General Guidelines

To check what's going on in the project, check:

- the [changelog](CHANGELOG.md)
- the [Captain's Log](CAPTAIN.LOG.md)
- the [Waffle board](https://waffle.io/ipfs/ipfs-cluster)
- the [Roadmap](ROADMAP.md)
- the [upcoming release issues](https://github.com/ipfs/ipfs-cluster/issues?q=label%3Arelease)


If you need help:

- open an issue
- ask on the `#ipfs-cluster` IRC channel (Freenode)


## Code contribution guidelines

* ipfs-cluster uses the MIT license.
* All contributions are via Pull Request, which needs a Code Review approval from one of the project collaborators.
* Tests must pass
* Code coverage must be stable or increase
* We prefer meaningul branch names: `feat/`, `fix/`...
* We prefer commit messages which reference an issue `fix #999: ...`
* The commit message should end with the following trailers :

  ```
  License: MIT
  Signed-off-by: User Name <email@address>
  ```

  where "User Name" is the author's real (legal) name and
  email@address is one of the author's valid email addresses.

  These trailers mean that the author agrees with the 
  [developer certificate of origin](docs/developer-certificate-of-origin)
  and with licensing the work under the [MIT license](docs/LICENSE).

  To help you automatically add these trailers, you can run the
  [setup_commit_msg_hook.sh](https://raw.githubusercontent.com/ipfs/community/master/dev/hooks/setup_commit_msg_hook.sh)
  script which will setup a Git commit-msg hook that will add the above
  trailers to all the commit messages you write.


These are just guidelines. We are friendly people and are happy to help :)
