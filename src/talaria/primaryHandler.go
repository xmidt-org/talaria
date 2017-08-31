package main

import (
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
)

func NewPrimaryHandler(logger log.Logger, connectedUpdates <-chan []byte, manager device.Manager, v *viper.Viper) (http.Handler, error) {
	poolFactory, err := wrp.NewPoolFactory(v.Sub(wrp.ViperKey))
	if err != nil {
		return nil, err
	}

	var (
		handler    = mux.NewRouter()
		apiHandler = handler.PathPrefix(baseURI + "/" + version).Subrouter()
	)

	apiHandler.Handle("/device/send", &device.MessageHandler{
		Logger:   logger,
		Decoders: poolFactory.NewDecoderPool(wrp.JSON),
		Router:   manager,
	}).
		Methods("POST", "PATCH").
		Headers("Content-Type", wrp.JSON.ContentType())

	apiHandler.Handle("/device/send", &device.MessageHandler{
		Logger:   logger,
		Decoders: poolFactory.NewDecoderPool(wrp.Msgpack),
		Router:   manager,
	}).
		Methods("POST", "PATCH").
		Headers("Content-Type", wrp.Msgpack.ContentType())

	listHandler := new(device.ListHandler)
	listHandler.Consume(connectedUpdates)
	apiHandler.Handle("/devices", listHandler).
		Methods("GET")

	apiHandler.Handle(
		"/device",
		device.UseID.FromHeader(&device.ConnectHandler{
			Logger:    logger,
			Connector: manager,
		}),
	)

	return handler, nil
}
