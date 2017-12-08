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
	"github.com/Comcast/webpa-common/secure"
	"github.com/Comcast/webpa-common/secure/handler"
	"github.com/Comcast/webpa-common/secure/key"
	"github.com/Comcast/webpa-common/wrp"
	"github.com/go-kit/kit/log"
	"github.com/gorilla/mux"
	"github.com/SermoDigital/jose/jwt"
	"github.com/spf13/viper"
)

const (
	baseURI = "/api"

	// TODO: Should this change for talaria 2.0?
	version = "v2"

	// TODO: This should be configurable at some point
	poolSize = 1000
	
	DefaultKeyId = "current"
)

type JWTValidator struct {
	// JWTKeys is used to create the key.Resolver for JWT verification keys
	Keys key.ResolverFactory `json:"keys"`

	// Custom is an optional configuration section that defines
	// custom rules for validation over and above the standard RFC rules.
	Custom secure.JWTValidatorFactory `json:"custom"`
}

func NewPrimaryHandler(logger log.Logger, manager device.Manager, v *viper.Viper) (http.Handler, error) {
	var (
		authKey                      = v.GetString("inbound.authKey")
		r                            = mux.NewRouter()
		apiHandler                   = r.PathPrefix(fmt.Sprintf("%s/%s", baseURI, version)).Subrouter()
		authorizationDecorator       = func(h http.Handler) http.Handler { return h }
		authorizationDecoratorDevice = func(h http.Handler) http.Handler { return h }
	)

	if len(authKey) > 0 {
		authorizationDecorator = handler.AuthorizationHandler{
			Logger:    logger,
			Validator: secure.ExactMatchValidator(authKey),
		}.Decorate
	}

	if v.IsSet("jwtValidators") {
		var validator secure.Validator
		var cfg_validators []JWTValidator

		if err := v.UnmarshalKey("jwtValidators", &cfg_validators); err != nil {
			validator = make(secure.Validators, 0, 0)
		} else {
			validators := make(secure.Validators, 0, len(cfg_validators))
			
			for _, validatorDescriptor := range cfg_validators {
				keyResolver, err := validatorDescriptor.Keys.NewResolver()
				if err != nil {
					return nil, err
				}

				validators = append(
					validators,
					secure.JWSValidator{
						DefaultKeyId:  DefaultKeyId,
						Resolver:      keyResolver,
						JWTValidators: []*jwt.Validator{validatorDescriptor.Custom.New()},
					},
				)
			}

			validator = validators
		}

		authorizationDecoratorDevice = handler.AuthorizationHandler{
			Logger:    logger,
			Validator: validator,
		}.Decorate
	}

	apiHandler.Handle("/device/send", authorizationDecorator(&device.MessageHandler{
		Logger:   logger,
		Decoders: wrp.NewDecoderPool(poolSize, wrp.JSON),
		Router:   manager,
	})).
		Methods("POST", "PATCH").
		Headers("Content-Type", wrp.JSON.ContentType())

	apiHandler.Handle("/device/send", authorizationDecorator(&device.MessageHandler{
		Logger:   logger,
		Decoders: wrp.NewDecoderPool(poolSize, wrp.Msgpack),
		Router:   manager,
	})).
		Methods("POST", "PATCH").
		Headers("Content-Type", wrp.Msgpack.ContentType())

	apiHandler.Handle("/devices", authorizationDecorator(&device.ListHandler{
		Logger:   logger,
		Registry: manager,
	})).
		Methods("GET")

	// the connect handler decorated for authorization
	apiHandler.Handle(
		"/device",
		authorizationDecoratorDevice(
			device.UseID.FromHeader(&device.ConnectHandler{
				Logger:    logger,
				Connector: manager,
			}),
		),
	).HeadersRegexp("Authorization", ".*")

	// the connect handler is not decorated for authorization
	apiHandler.Handle(
		"/device",
		device.UseID.FromHeader(&device.ConnectHandler{
			Logger:    logger,
			Connector: manager,
		}),
	)

	apiHandler.Handle(
		"/device/{deviceID}/stat",
		authorizationDecorator(&device.StatHandler{
			Logger:   logger,
			Registry: manager,
			Variable: "deviceID",
		}),
	)

	return r, nil
}
