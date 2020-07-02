# Codesearch As A Service

This is a set of changes built on top of the code found in the
https://github.com/google/codesearch repository to turn what was local indexing
and search tools into a service.

## Getting Started

Right now things are in a rough state of implementation, with likely poor
debugability and rough corners. To get started you will need to do something
like the following:

```shell
$ go build ./cmd/cindex-git ./cmd/cindex-serve ./cmd/csearch-ui
# Default cache size is 64MiB - might want to make it bigger if a lot of content
# is indexed.
$ ./cindex-serve -port 8801 -cache_size $$((64*1024*1024))
# Now to index some stuff. `-repo` is alocal git repo used for cloning and
# speeding up incremental indexing. `-url` is where to fetch the repo for
# indexing from. `-ref` is the full repository ref to index.
$ ./cindex-git -repo ~/index-repo -url git@github.com:nairb774/codesearch -ref refs/heads/master
# Repeat as many times as needed, configure a cron, ...
# Start the UI to actually search:
$ ./csearch-ui -port 8800
# Visit http://localhost:8800 to search.
```

Index data is stored under `os.UserConfigDir()/codesearch` which differs per OS.
See index2/paths.go for details.

## Warnings

The system can handle indexing the Linux Kernel, Kubernetes and other large
"mono-repos" concurrently, but getting the system to "perform" and not trigger
errors may not be trivial at this point without knowledge of internal details.

The current UI has no security protections. It is entirely possible that someone
could exfiltrate all of the code indexed on your machine. Run with care.
