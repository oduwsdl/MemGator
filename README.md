# MemGator

A Memento Aggregator CLI and Server in [Go](https://golang.org/).

## Features

* The binary (available for various platforms) can be used as the CLI or run as a Web Service
* Results available in three formats - Link/JSON/CDXJ
* TimeMap, TimeGate, TimeNav, and Redirect endpoints
* Good API parity with the [main Memento Aggregator service](http://timetravel.mementoweb.org/guide/api/)
* Concurrent - Splits every session in subtasks for parallel execution
* Parallel - Utilizes all the available CPUs
* Custom archive list (a local JSON file or a remote URL) - a sample JSON is included in the repository
* Probability based archive prioritization and limit
* Three levels of customizable timeouts for greater control over remote requests
* Customizable logging and profiling in CDXJ format
* Customizable endpoint URLs - helpful in load-balancing
* Customizable User-Agent to be sent to each archive and User-Agent spoofing
* Configurable archive failure detection and automatic hibernation
* [CORS](http://www.w3.org/TR/cors/) support to make it easy to use it from JavaScript clients
* Memento count exposed in the header that can be retrieved via `HEAD` request
* [Docker](https://www.docker.com/) friendly - An image available as [ibnesayeed/memgator](https://hub.docker.com/r/ibnesayeed/memgator/)
* Sensible defaults - Batteries included, but replaceable

## Usage

### CLI

Command line interface of MemGator allows retrieval of TimeMap and TimeGate over `STDOUT` in all supported formats. Info/Profiling (in verbose mode) and Error output is available on `STDERR` unless appropriate files are configured. For further details, see the full usage.

```
$ memgator [options] {URI-R}                            # TimeMap CLI
$ memgator [options] {URI-R} {YYYY[MM[DD[hh[mm[ss]]]]]} # TimeGate CLI
```

### Server

When run as a Web Service, MemGator exposes four customizable endpoints as follows:

```
$ memgator [options] server
TimeMap  : http://localhost:1208/timemap/link|json|cdxj/{URI-R}
TimeGate : http://localhost:1208/timegate/{URI-R} [Accept-Datetime]
TimeNav  : http://localhost:1208/timenav/link|json|cdxj/{YYYY[MM[DD[hh[mm[ss]]]]]}/{URI-R}
Redirect : http://localhost:1208/redirect/{YYYY[MM[DD[hh[mm[ss]]]]]}/{URI-R}
```

The `TimeMap` and `TimeGate` responses are in accordance with the [Memento RFC](http://tools.ietf.org/html/rfc7089). Additionally, the TimeMap endpoint also supports some additional serialization formats. The `TimeNav` service is a URL friendly way to expose the same information in the response body (in various formats) as available in the `Link` header of the `TimeGate` response without the need of a header based time negotiation. The `Redirect` service resolves the datetime (full or partial) passed in the URL and redirects to the closest Memento.

## Download and Install

Depending on the machine and operating system download appropriate binary from the [releases page](https://github.com/oduwsdl/memgator/releases). Changed the mode of the file to executable `chmod +x MemGator-BINARY`. Run from the current location of the downloaded binary or rename it to `memgator` and move it into a directory that is in the `PATH` (such as `/usr/local/bin/`).

## Running as a Docker Container

The first command below is not necessary, but it allows pulling the latest version of the MemGator Docker image.

```
$ docker pull ibnesayeed/memgator
$ docker run ibnesayeed/memgator -h
$ docker run ibnesayeed/memgator [options] {URI-R}
$ docker run ibnesayeed/memgator [options] {URI-R} {YYYY[MM[DD[hh[mm[ss]]]]]}
$ docker run ibnesayeed/memgator [options] server
```

### Full Usage

```
MemGator {Version}
   _____                  _______       __
  /     \  _____  _____  / _____/______/  |___________
 /  Y Y  \/  __ \/     \/  \  ___\__  \   _/ _ \_   _ \
/   | |   \  ___/  Y Y  \   \_\  \/ __ |  | |_| |  | \/
\__/___\__/\____\__|_|__/\_______/_____|__|\___/|__|

Usage:
  memgator [options] {URI-R}                                  # TimeMap CLI
  memgator [options] {URI-R} {YYYY[MM[DD[hh[mm[ss]]]]]}       # TimeGate CLI
  memgator [options] server                                   # Run as a Web Service

Options:
  -A, --agent=MemGator:{Version} <{CONTACT}>                  User-agent string sent to archives
  -a, --arcs=http://oduwsdl.github.io/memgator/archives.json  Local/remote JSON file path/URL for list of archives
  -c, --contact=@WebSciDL                                     Email/URL/Twitter handle - Used in the user-agent
  -d, --dormant=15m0s                                         Dormant period after consecutive failures
  -F, --tolerance=-1                                          Failure tolerance limit for each archive
  -f, --format=Link                                           Output format - Link/JSON/CDXJ
  -g, --timegate=http://{SERVICE}/timegate                    TimeGate base URL - default based on service URL
  -H, --host=localhost                                        Host name - only used in web service mode
  -k, --topk=-1                                               Aggregate only top k archives based on probability
  -l, --log=                                                  Log file location - Defaults to STDERR
  -m, --timemap=http://{SERVICE}/timemap                      TimeMap base URL - default based on service URL
  -P, --profile=                                              Profile file location - Defaults to Logfile
  -p, --port=1208                                             Port number - only used in web service mode
  -r, --restimeout=1m0s                                       Response timeout for each archive
  -S, --spoof=false                                           Spoof each request with a random user-agent
  -s, --service=http://{HOST}[:{PORT}]                        Service base URL - default based on host & port
  -T, --hdrtimeout=30s                                        Header timeout for each archive
  -t, --contimeout=5s                                         Connection timeout for each archive
  -V, --verbose=false                                         Show Log and Profiling messages on STDERR
  -v, --version=false                                         Show name and version
```

## Build

Assuming that Git and Go (version >= 1.3) are installed, the `GOPATH` environment variable is set to the Go Workspace directory as described in the [How to Write Go Code](https://golang.org/doc/code.html), and `PATH` includes `$GOPATH/bin`. Cloning, building, and running the code can be done using following commands:

```
$ cd $GOPATH
$ go get github.com/oduwsdl/memgator
$ go install github.com/oduwsdl/memgator
$ memgator http://example.com/
```

To compile cross-platform binaries, go to the MemGator source directory and run the `crossbuild.sh` script:

```
$ cd $GOPATH/src/github.com/oduwsdl/memgator
$ ./crossbuild.sh
```

This will generate various binaries in `/tmp/mgbins` directory.
