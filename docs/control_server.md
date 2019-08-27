---
title: Control Talaria Server
sort_rank: 2
---

Talaria exposes a built-in control server that can be used to adjust certain features.

## Device Gate
Talaria can be set to disallow incoming websocket connections.  When the gate is closed, all incoming websocket connection requests are rejected with a **503** status.  Talaria always starts with the gate open, allowing new websocket connections.  Already connected websockets are not affected by closing the gate.

The RESTful endpoint that controls this is `/api/v2/device/gate`.  Note that the port for the control server is not the same as the port for the websocket server.

* `GET host:port/api/v2/device/gate` returns a JSON message indicating the status of the gate.  For example, `{"open": true, "timestamp": "2009-11-10T23:00:00Z"}` indicates that the gate is open and has been open since the given timestamp.  Similarly, `"open": false` indicates that the gate is closed.
* `POST/PUT/PATCH host:port/api/v2/device/gate?open=<boolean>` raises or lowers the gate.  The value of the open parameter may be anything that `golang` will parse as a boolean (see [ParseBool](https://godoc.org/strconv#ParseBool)).  This endpoint is idempotent.  Any attempt to open the gate when it is already open or close it when it is already closed results in a **200** status.  If this endpoint did change the status of the gate, a **201** status is returned.

### Metrics

`xmidt_talaria_gate_status` is the exposed Prometheus metric that indicates the status of the gate.  When this gauge is 0.0, the gate is closed.  When this gauge is 1.0, the gate is open.

## Connection Drain
Talaria supports the draining of websocket connections.  Essentially, this means shedding load in a controlled fashion.  Websocket connections can be drained at a given rate, e.g. `100 connections/minute`, or can be drained as fast as possible.

Only (1) drain job is permitted to be running at any time.  Attempts to start a drain when one is already active results in an error.

**IMPORTANT**:  The device gate may be open when a drain is started.  That means that devices can connect and disconnect during a drain.  In order to prevent a situation where the drain job cannot ever complete, computations about the number of devices are done once when the job is started.  For example, if a drain job is instructed to drain all devices, the count of devices is computed at the start of the job.  This may mean that connections are left at the end of a drain.  If this behavior is not desired, *close the device gate before starting a drain.*

The RESTful endpoint that controls the connection drain is `/api/v2/device/drain`.  Note that the port for the control server is not the same as the port for the websocket server.

* `GET host:port/api/v2/device/drain` returns a JSON message indicating whether a drain job is active and the progress of the active job if one is running.  If a drain has previously completed, the information about that job will be available via this endpoint until a new drain job is started.

* `POST/PUT/PATCH host:port/api/v2/device/drain` attempts to start a drain job.  This endpoint returns the same JSON message as a `GET` when it starts a drain job, along with a **200** status.  If a drain job is already running, this endpoint returns a **429 Conflict** status.  If no parameters are supplied, all devices are drained as fast as possible.  Several parameters may be supplied to customize the drain:

    + `count`: The maximum number of websocket connections to close.  If this value is larger than the number of connections at the start of the job, the current count of connections is used instead.
    + `percent`: The percentage of connections to close.  The computation of how many connections is done once when the job is started.  If both `count` and `percent` are supplied, `count` is ignored.
    + `rate`: The number of connections per unit time (tick) to close.  If this value is not supplied, connections are closed as fast as possible.
    + `tick`: The unit of time for `rate`.  If `rate` is supplied and `tick` is not supplied, a `tick` of `1s` is used.  If `rate` is not supplied and `tick` is supplied, `tick` is ignored.  The value of `tick` may be anything parseable by the `golang` standard library (see [ParseDuration](https://godoc.org/time#ParseDuration)).  

* `DELETE host:port/api/v2/device/drain` attempts to cancel any running drain job.  Note that a running job may not cancel immediately.  If no drain job is running, **429 Conflict** is returned.

### Metrics

Two Prometheus metrics are exposed to monitor the drain feature:

* `xmidt_talaria_drain_count` is the total number of connections that were closed due to a drain since the server was started.
* `xmidt_talaria_drain_status` is a gauge indicating whether a drain is running.  This gauge will be `0.0` when no drain job is running, and `1.0` when a drain job is active.
