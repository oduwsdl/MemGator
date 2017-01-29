# MemGator

A Memento Aggregator CLI and Server in [Go](https://golang.org/).

## Features

* The binary (available for various platforms) can be used as the CLI or run as a Web Service
* Results available in three formats - Link/JSON/CDXJ
* TimeMap, TimeGate, and Memento (redirect or description) endpoints
* Optional streaming of benchmarks over [Server-Sent Events](http://www.html5rocks.com/en/tutorials/eventsource/basics/) (SSE) for realtime visualization and monitoring
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

Command line interface of MemGator allows retrieval of the TimeMap and the description of the closest Memento (equivalent to the TimeGate) over `STDOUT` in all supported formats. Logs and benchmarks (in verbose mode) and Error output are available on `STDERR` unless appropriate files are configured. For further details, see the full usage.

```
$ memgator [options] {URI-R}                            # TimeMap from CLI
$ memgator [options] {URI-R} {YYYY[MM[DD[hh[mm[ss]]]]]} # Description of the closest Memento from CLI
```

### Server

When run as a Web Service, MemGator exposes following customizable endpoints:

```
$ memgator [options] server
TimeMap   : http://localhost:1208/timemap/{FORMAT}/{URI-R}
TimeGate  : http://localhost:1208/timegate/{URI-R} [Accept-Datetime]
Memento   : http://localhost:1208/memento[/{FORMAT}|proxy]/{DATETIME}/{URI-R}
Benchmark : http://localhost:1208/monitor [SSE]

# FORMAT          => link|json|cdxj
# DATETIME        => YYYY[MM[DD[hh[mm[ss]]]]]
# Accept-Datetime => Header in RFC1123 format
```

* `TimeMap` endpoint serves an aggregated TimeMap for a given URI-R in accordance with the [Memento RFC](http://tools.ietf.org/html/rfc7089). Additionally, it makes sure that the Mementos are chronologically ordered. It also provides the TimeMap data serialized in additional experimental formats.
* `TimeGate` endpoint allows datetime negotiation via the `Accept-Datetime` header in accordance with the [Memento RFC](http://tools.ietf.org/html/rfc7089). A successful response redirects to the closes Memento (to the given datetime) using the `Location` header. The default datetime is the current time. A successful response also includes a `Link` header which provides links to the first, last, next, and previous Mementos.
* `Memento` endpoint allows datetime negotiation in the request URL itself for clients that cannot easily send custom request headers (as opposed to the `TimeGate` which requires the `Accept-Datetime` header). This endpoint behaves differently based on whether the `format` was specified in the request. It essentially splits the functionality of the `TimeGate` endpoint as follows:
 * If a format is specified, it returns the description of the closest Memento (to the given datetime) in the specified format. It is essentially the same data that is available in the `Link` header of the `TimeGate` response, but as the payload in the format requested by the client.
 * If a format is not specified, it redirects to the closest Memento (to the given datetime) using the `Location` header.
 * If the term `proxy` is used instead of a format then it acts like a proxy for the closest original unmodified Memento with added CORS headers.
* `Benchmark` is an optional endpoint that can be enabled by the `--monitor` flag when the server is started. If enabled, it provides a stream of the benchmark log over [SSE](http://www.html5rocks.com/en/tutorials/eventsource/basics/) for realtime visualization and monitoring.

**NOTE:** A fallback endpoint `/api` is added for compatibility with [Time Travel APIs](http://timetravel.mementoweb.org/guide/api/#memento-json) to allow drop-in replacement in existing tools. This endpoint is an alias to the `/memento` endpoint that returns the description of a Memento.

## Download and Install

Depending on the machine and operating system download appropriate binary from the [releases page](https://github.com/oduwsdl/memgator/releases). Change the mode of the file to executable `chmod +x MemGator-BINARY`. Run from the current location of the downloaded binary or rename it to `memgator` and move it into a directory that is in the `PATH` (such as `/usr/local/bin/`) to make it available as a command.

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
  memgator [options] {URI-R}                            # TimeMap from CLI
  memgator [options] {URI-R} {YYYY[MM[DD[hh[mm[ss]]]]]} # Description of the closest Memento from CLI
  memgator [options] server                             # Run as a Web Service

Options:
  -A, --agent=MemGator:{Version} <{CONTACT}>  User-agent string sent to archives
  -a, --arcs=http://git.io/archives           Local/remote JSON file path/URL for list of archives
  -b, --benchmark=                            Benchmark file location - Defaults to Logfile
  -c, --contact=@WebSciDL                     Email/URL/Twitter handle - used in the user-agent
  -D, --static=                               Directory path to serve static assets from
  -d, --dormant=15m0s                         Dormant period after consecutive failures
  -F, --tolerance=-1                          Failure tolerance limit for each archive
  -f, --format=Link                           Output format - Link/JSON/CDXJ
  -H, --host=localhost                        Host name - only used in web service mode
  -k, --topk=-1                               Aggregate only top k archives based on probability
  -l, --log=                                  Log file location - defaults to STDERR
  -m, --monitor=false                         Benchmark monitoring via SSE
  -P, --proxy=http://{HOST}[:{PORT}]{ROOT}    Proxy URL - defaults to host, port, and root
  -p, --port=1208                             Port number - only used in web service mode
  -R, --root=/                                Service root path prefix
  -r, --restimeout=1m0s                       Response timeout for each archive
  -S, --spoof=false                           Spoof each request with a random user-agent
  -T, --hdrtimeout=30s                        Header timeout for each archive
  -t, --contimeout=5s                         Connection timeout for each archive
  -V, --verbose=false                         Show Info and Profiling messages on STDERR
  -v, --version=false                         Show name and version
```

## Build

Assuming that Git and Go (version >= 1.7) are installed, the `GOPATH` environment variable is set to the Go Workspace directory as described in the [How to Write Go Code](https://golang.org/doc/code.html), and `PATH` includes `$GOPATH/bin`. Cloning, building, and running the code can be done using following commands:

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
