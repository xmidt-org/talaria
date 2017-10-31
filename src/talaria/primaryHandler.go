package main

import (
	"fmt"
	"net/http"

	"github.com/Comcast/webpa-common/device"
	"github.com/Comcast/webpa-common/wrp"
	"github.com/go-kit/kit/log"
	"github.com/gorilla/mux"
	"github.com/spf13/viper"
)

const (
	baseURI = "/api"

	// TODO: Should this change for talaria 2.0?
	version = "v2"

	// TODO: This should be configurable at some point
	poolSize = 1000
)

func NewPrimaryHandler(logger log.Logger, manager device.Manager, v *viper.Viper) (http.Handler, error) {
	var (
		handler    = mux.NewRouter()
		apiHandler = handler.PathPrefix(fmt.Sprintf("%s/%s", baseURI, version)).Subrouter()
	)

	apiHandler.Handle("/device/send", &device.MessageHandler{
		Logger:   logger,
		Decoders: wrp.NewDecoderPool(poolSize, wrp.JSON),
		Router:   manager,
	}).
		Methods("POST", "PATCH").
		Headers("Content-Type", wrp.JSON.ContentType())

	apiHandler.Handle("/device/send", &device.MessageHandler{
		Logger:   logger,
		Decoders: wrp.NewDecoderPool(poolSize, wrp.Msgpack),
		Router:   manager,
	}).
		Methods("POST", "PATCH").
		Headers("Content-Type", wrp.Msgpack.ContentType())

	apiHandler.Handle("/devices", &device.ListHandler{
		Logger:   logger,
		Registry: manager,
	}).
		Methods("GET")

	apiHandler.Handle(
		"/device",
		device.UseID.FromHeader(&device.ConnectHandler{
			Logger:    logger,
			Connector: manager,
		}),
	)

	apiHandler.Handle(
		"/device/{deviceID}/stat",
		&device.StatHandler{
			Logger:   logger,
			Registry: manager,
			Variable: "deviceID",
		},
	)

	return handler, nil
}
