package main

import (
	"fmt"
	"net/http"

	"github.com/Comcast/webpa-common/logging"
	"github.com/Comcast/webpa-common/logging/logginghttp"
	"github.com/Comcast/webpa-common/xhttp"
	"github.com/Comcast/webpa-common/xhttp/gate"
	"github.com/Comcast/webpa-common/xhttp/xcontext"
	"github.com/Comcast/webpa-common/xmetrics"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/gorilla/mux"
	"github.com/spf13/viper"
)

const (
	ControlKey = "control"
)

func StartControlServer(logger log.Logger, registry xmetrics.Registry, v *viper.Viper) (func(http.Handler) http.Handler, error) {
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

		r          = mux.NewRouter()
		apiHandler = r.PathPrefix(fmt.Sprintf("%s/%s", baseURI, version)).Subrouter()
	)

	apiHandler.Handle("/device/gate", &gate.Lever{Gate: g, Parameter: "open"}).
		Methods("POST", "PUT", "PATCH")

	apiHandler.Handle("/device/gate", &gate.Status{Gate: g}).
		Methods("GET")

	server := xhttp.NewServer(options)
	server.Handler = xcontext.Populate(0, logginghttp.SetLogger(logger))(r)

	starter := xhttp.NewStarter(options.StartOptions(), server)
	go func() {
		if err := starter(); err != nil {
			logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), "Unable to start control server", logging.ErrorKey(), err)
		}
	}()

	return gate.NewConstructor(g), nil
}
