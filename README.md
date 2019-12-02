# talaria
(pronounced "tuh-laa-ree-uh")

[![Build Status](https://travis-ci.com/xmidt-org/talaria.svg?branch=master)](https://travis-ci.com/xmidt-org/talaria)
[![codecov.io](http://codecov.io/github/xmidt-org/talaria/coverage.svg?branch=master)](http://codecov.io/github/xmidt-org/talaria?branch=master)
[![Code Climate](https://codeclimate.com/github/xmidt-org/talaria/badges/gpa.svg)](https://codeclimate.com/github/xmidt-org/talaria)
[![Issue Count](https://codeclimate.com/github/xmidt-org/talaria/badges/issue_count.svg)](https://codeclimate.com/github/xmidt-org/talaria)
[![Go Report Card](https://goreportcard.com/badge/github.com/xmidt-org/talaria)](https://goreportcard.com/report/github.com/xmidt-org/talaria)
[![Apache V2 License](http://img.shields.io/badge/license-Apache%20V2-blue.svg)](https://github.com/xmidt-org/talaria/blob/master/LICENSE)
[![GitHub release](https://img.shields.io/github/release/xmidt-org/talaria.svg)](CHANGELOG.md)

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
* `make rpm`: builds an rpm containing Talaria
* `make docker`: builds a docker image for Talaria, making sure to get all
   dependencies
* `make local-docker`: builds a docker image for Talaria with the assumption
   that the dependencies can be found already
* `make test`: runs unit tests with coverage for Talaria
* `make clean`: deletes previously-built binaries and object files

### Docker

The docker image can be built either with the Makefile or by running a docker
command.  Either option requires first getting the source code.

See [Makefile](#Makefile) on specifics of how to build the image that way.

For running a command, either you can run `docker build` after getting all
dependencies, or make the command fetch the dependencies.  If you don't want to
get the dependencies, run the following command:
```bash
docker build -t talaria:local -f deploy/Dockerfile .
```
If you want to get the dependencies then build, run the following commands:
```bash
GO111MODULE=on go mod vendor
docker build -t talaria:local -f deploy/Dockerfile.local .
```

For either command, if you want the tag to be a version instead of `local`,
then replace `local` in the `docker build` command.

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
