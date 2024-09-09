// SPDX-FileCopyrightText: 2017 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/xmidt-org/candlelight"
	"github.com/xmidt-org/touchstone"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gorilla/mux/otelmux"
	"go.uber.org/zap"

	"github.com/gorilla/mux"
	"github.com/justinas/alice"
	"github.com/spf13/viper"
	"github.com/xmidt-org/webpa-common/v2/device"
	"github.com/xmidt-org/webpa-common/v2/device/devicegate"
	"github.com/xmidt-org/webpa-common/v2/device/drain"

	// nolint:staticcheck
	"github.com/xmidt-org/webpa-common/v2/xhttp"
	"github.com/xmidt-org/webpa-common/v2/xhttp/gate"
)

const (
	ControlKey = "control"
	gatePath   = "/device/gate"
	filterPath = "/device/gate/filter"
	drainPath  = "/device/drain"
)

func StartControlServer(logger *zap.Logger, manager device.Manager, deviceGate devicegate.Interface, tf *touchstone.Factory, v *viper.Viper, tracing candlelight.Tracing) (func(http.Handler) http.Handler, error) {
	if !v.IsSet(ControlKey) {
		return xhttp.NilConstructor, nil
	}

	var options xhttp.ServerOptions
	if err := v.UnmarshalKey(ControlKey, &options); err != nil {
		return xhttp.NilConstructor, err
	}

	options.Logger = logger

	var errs error
	gateStatus, err := tf.NewGauge(
		prometheus.GaugeOpts{
			Name: GateStatus,
			Help: "Indicates whether the device gate is open (1.0) or closed (0.0)",
		},
	)
	errs = errors.Join(errs, err)

	drainStatus, err := tf.NewGauge(
		prometheus.GaugeOpts{
			Name: DrainStatus,
			Help: "Indicates whether a device drain operation is currently running",
		},
	)
	errs = errors.Join(errs, err)

	drainCounter, err := tf.NewGauge(
		prometheus.GaugeOpts{
			Name: DrainCounter,
			Help: "The total count of devices disconnected due to a drain since the server started",
		},
	)
	errs = errors.Join(errs, err)
	if errs != nil {
		return xhttp.NilConstructor, err
	}

	var (
		g = gate.New(
			true,
			gate.WithGauge(gateStatus),
		)

		d = drain.New(
			drain.WithLogger(logger),
			drain.WithManager(manager),
			drain.WithStateGauge(drainStatus),
			drain.WithDrainCounter(drainCounter),
		)

		gateLogger    = devicegate.GateLogger{Logger: logger}
		filterHandler = &devicegate.FilterHandler{Gate: deviceGate}

		r          = mux.NewRouter()
		apiHandler = r.PathPrefix(fmt.Sprintf("%s/%s", baseURI, version)).Subrouter()
	)

	otelMuxOptions := []otelmux.Option{
		otelmux.WithPropagators(tracing.Propagator()),
		otelmux.WithTracerProvider(tracing.TracerProvider()),
	}

	r.Use(otelmux.Middleware("control", otelMuxOptions...), candlelight.EchoFirstTraceNodeInfo(tracing.Propagator(), true))

	apiHandler.Handle(gatePath, &gate.Lever{Gate: g, Parameter: "open"}).Methods("POST", "PUT", "PATCH")

	apiHandler.Handle(gatePath, &gate.Status{Gate: g}).Methods("GET")

	apiHandler.HandleFunc(filterPath, filterHandler.GetFilters).Methods("GET")

	apiHandler.Handle(filterPath, alice.New(gateLogger.LogFilters).Then(http.HandlerFunc(filterHandler.UpdateFilters))).Methods("POST", "PUT")

	apiHandler.Handle(filterPath, alice.New(gateLogger.LogFilters).Then(http.HandlerFunc(filterHandler.DeleteFilter))).Methods("DELETE")

	apiHandler.Handle(drainPath, &drain.Start{Drainer: d}).Methods("POST", "PUT", "PATCH")

	apiHandler.Handle(drainPath, &drain.Cancel{Drainer: d}).Methods("DELETE")

	apiHandler.Handle(drainPath, &drain.Status{Drainer: d}).Methods("GET")

	server := xhttp.NewServer(options)
	server.Handler = setLogger(logger)(r)

	starter := xhttp.NewStarter(options.StartOptions(), server)
	go func() {
		if err := starter(); err != nil {
			logger.Error("Unable to start control server", zap.Error(err))
		}
	}()

	return gate.NewConstructor(g), nil
}
