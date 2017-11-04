/**
 * Copyright 2017 Comcast Cable Communications Management, LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */
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
