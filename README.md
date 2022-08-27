# talaria
(pronounced "tuh-laa-ree-uh")

[![Build Status](https://github.com/xmidt-org/talaria/actions/workflows/ci.yml/badge.svg)](https://github.com/xmidt-org/talaria/actions/workflows/ci.yml)
[![codecov.io](http://codecov.io/github/xmidt-org/talaria/coverage.svg?branch=main)](http://codecov.io/github/xmidt-org/talaria?branch=main)
[![Go Report Card](https://goreportcard.com/badge/github.com/xmidt-org/talaria)](https://goreportcard.com/report/github.com/xmidt-org/talaria)
[![Quality Gate Status](https://sonarcloud.io/api/project_badges/measure?project=xmidt-org_talaria&metric=alert_status)](https://sonarcloud.io/dashboard?id=xmidt-org_talaria)
[![Apache V2 License](http://img.shields.io/badge/license-Apache%20V2-blue.svg)](https://github.com/xmidt-org/talaria/blob/main/LICENSE)
[![GitHub Release](https://img.shields.io/github/release/xmidt-org/talaria.svg)](CHANGELOG.md)

## Summary
Talaria's primary function is to interact with the devices:
forwarding device events and sending requests to the device then forwarding the response.
The communication with the device happens over a websocket
using [WRP Messages](https://github.com/xmidt-org/wrp-c/wiki/Web-Routing-Protocol).

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Details](#details)
- [Build](#build)
- [Deploy](#deploy)
- [Contributing](#contributing)

## Code of Conduct

This project and everyone participating in it are governed by the [XMiDT Code Of Conduct](https://xmidt.io/code_of_conduct/). 
By participating, you agree to this Code.

## Details

### Device Interaction
Talaria's primary function is to interact with the devices.
The communication with the device happens over a websocket
using [WRP Messages](https://github.com/xmidt-org/wrp-c/wiki/Web-Routing-Protocol).

Talaria can send events originating from the device as well as emit its own events.
When this occurs, the event is sent to [Caduceus](https://github.com/xmidt-org/caduceus).

Talaria has three API endpoints to interact with the devices connected to itself.
A [XMiDT](https://xmidt.io/) cloud client should not directly query against a talaria.
Instead, they should send a request through [scytale](https://github.com/xmidt-org/scytale).

#### Device Statistics - `/device/{deviceID}/stat` endpoint
This will return the statistics of the connected device,
including information such as uptime and bytes sent.
This request does not communicate with the device, instead the request returns
stored statistics.

#### Get Devices - `/devices` endpoint
This will return a list of all the actively connected devices and their statistics,
just like the `stat` command.

:warning: _Warning_: this is an expensive request. Use with caution.
This is not recommended to be used in production.

#### Send WRP to Device - `/device/send` endpoint
This will send a WRP message to the device.
Talaria will accept a WRP message encoded in a valid WRP representation - generally `msgpack` or `json`.
If the message is `json` encoded, talaria will encode the payload as `msgpack`.
Talaria will then forward the message to the device.
If the device returns a message, it will be encoded as the HTTP `accept` header.
`msgpack` is the default encoding of the wrp message.

### Control Devices
A secondary function of talaria is to control the connected devices. This allows
for the flow of devices to go towards specific talarias. In other words, where the
websockets are made can be controlled.
For more information refer to [Control Server Docs](docs/control_server.md).

#### Gate Devices - `/device/gate` endpoint
This will allow or deny devices to connect to the talaria instance.

#### Drain Devices - `/device/drain` endpoint
This will remove the connected devices from the talaria instance.

## Build

### Source

In order to build from the source, you need a working Go environment with
version 1.11 or greater. Find more information on the [Go website](https://golang.org/doc/install).

You can directly use `go get` to put the Talaria binary into your `GOPATH`:
```bash
GO111MODULE=on go get github.com/xmidt-org/talaria
```

You can also clone the repository yourself and build using make:

```bash
mkdir -p $GOPATH/src/github.com/xmidt-org
cd $GOPATH/src/github.com/xmidt-org
git clone git@github.com:xmidt-org/talaria.git
cd talaria
make build
```

### Makefile

The Makefile has the following options you may find helpful:
* `make build`: builds the Talaria binary
* `make docker`: fetches all dependencies from source and builds a 
   Talaria docker image
* `make local-docker`: vendors dependencies and builds a Talaria docker image (recommended for local testing)
* `make test`: runs unit tests with coverage for Talaria
* `make clean`: deletes previously-built binaries and object files

### RPM

First have a local clone of the source and go into the root directory of the 
repository.  Then use rpkg to build the rpm:
```bash
rpkg srpm --spec <repo location>/<spec file location in repo>
rpkg -C <repo location>/.config/rpkg.conf sources --outdir <repo location>'
```

### Docker

The docker image can be built either with the Makefile or by running a docker
command.  Either option requires first getting the source code.

See [Makefile](#Makefile) on specifics of how to build the image that way.

If you'd like to build it without make, follow these instructions based on your use case:

- Local testing
```bash
go mod vendor
docker build -t talaria:local -f deploy/Dockerfile .
```
This allows you to test local changes to a dependency. For example, you can build 
a Talaria image with the changes to an upcoming changes to [webpa-common](https://github.com/xmidt-org/webpa-common) by using the [replace](https://golang.org/ref/mod#go) directive in your go.mod file like so:
```
replace github.com/xmidt-org/webpa-common v1.10.8 => ../webpa-common
```
**Note:** if you omit `go mod vendor`, your build will fail as the path `../webpa-common` does not exist on the builder container.

- Building a specific version
```bash
git checkout v0.5.7 
docker build -t talaria:v0.5.7 -f deploy/Dockerfile .
```

**Additional Info:** If you'd like to stand up a XMiDT docker-compose cluster, read [this](https://github.com/xmidt-org/xmidt/blob/master/deploy/docker-compose/README.md).

### Kubernetes

A helm chart can be used to deploy talaria to kubernetes
```
helm install xmidt-talaria deploy/helm/talaria/
```

## Deploy

For deploying a XMiDT cluster refer to [getting started](https://xmidt.io/docs/operating/getting_started/).

For running locally, ensure you have the binary [built](#Source).  If it's in
your `GOPATH`, run:
```
talaria
```
If the binary is in your current folder, run:
```
./talaria
```

## Contributing

Refer to [CONTRIBUTING.md](CONTRIBUTING.md).
