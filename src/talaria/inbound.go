package main

import (
	"github.com/Comcast/webpa-common/device"
	"github.com/Comcast/webpa-common/logging"
	"github.com/Comcast/webpa-common/wrp"
	"github.com/gorilla/mux"
	"github.com/spf13/viper"
	"net/http"
)

func NewInboundHandler(logger logging.Logger, manager device.Manager, v *viper.Viper) (http.Handler, error) {
	poolFactory, err := wrp.NewPoolFactory(v.Sub(wrp.ViperKey))
	if err != nil {
		return nil, err
	}

	handler := mux.NewRouter()
	handler.Handle("/device", device.NewTranscodingHandler(poolFactory.NewDecoderPool(wrp.JSON), manager)).
		Methods("POST").
		Headers("Content-Type", "application/json")

	handler.Handle("/device", device.NewMsgpackHandler(poolFactory.NewDecoderPool(wrp.Msgpack), manager)).
		Methods("POST").
		Headers("Content-Type", "application/wrp")

	handler.Handle("/connect", device.NewConnectHandler(manager, nil, logger))
	return handler, nil
}
