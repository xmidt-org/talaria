package main

import (
	"fmt"
	"net/http"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/gorilla/mux"
	"github.com/spf13/viper"
	"github.com/xmidt-org/webpa-common/device"
	"github.com/xmidt-org/webpa-common/device/drain"
	"github.com/xmidt-org/webpa-common/logging"
	"github.com/xmidt-org/webpa-common/logging/logginghttp"
	"github.com/xmidt-org/webpa-common/xhttp"
	"github.com/xmidt-org/webpa-common/xhttp/gate"
	"github.com/xmidt-org/webpa-common/xhttp/xcontext"
	"github.com/xmidt-org/webpa-common/xmetrics"
)

const (
	ControlKey = "control"
)

func StartControlServer(logger log.Logger, manager device.Manager, registry xmetrics.Registry, v *viper.Viper) (func(http.Handler) http.Handler, error) {
	if !v.IsSet(ControlKey) {
		return xhttp.NilConstructor, nil
	}

	var options xhttp.ServerOptions
	if err := v.UnmarshalKey(ControlKey, &options); err != nil {
		return xhttp.NilConstructor, err
	}

	options.Logger = logger

	var (
		g = gate.New(
			true,
			gate.WithGauge(registry.NewGauge(GateStatus)),
		)

		d = drain.New(
			drain.WithLogger(logger),
			drain.WithManager(manager),
			drain.WithStateGauge(registry.NewGauge(DrainStatus)),
			drain.WithDrainCounter(registry.NewCounter(DrainCounter)),
		)

		r          = mux.NewRouter()
		apiHandler = r.PathPrefix(fmt.Sprintf("%s/%s", baseURI, version)).Subrouter()
	)

	apiHandler.Handle("/device/gate", &gate.Lever{Gate: g, Parameter: "open"}).
		Methods("POST", "PUT", "PATCH")

	apiHandler.Handle("/device/gate", &gate.Status{Gate: g}).
		Methods("GET")

	apiHandler.Handle("/device/drain", &drain.Start{d}).
		Methods("POST", "PUT", "PATCH")

	apiHandler.Handle("/device/drain", &drain.Cancel{d}).
		Methods("DELETE")

	apiHandler.Handle("/device/drain", &drain.Status{d}).
		Methods("GET")

	server := xhttp.NewServer(options)
	server.Handler = xcontext.Populate(logginghttp.SetLogger(logger))(r)

	starter := xhttp.NewStarter(options.StartOptions(), server)
	go func() {
		if err := starter(); err != nil {
			logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), "Unable to start control server", logging.ErrorKey(), err)
		}
	}()

	return gate.NewConstructor(g), nil
}
