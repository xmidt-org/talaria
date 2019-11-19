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
	"time"

	"github.com/SermoDigital/jose/jwt"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/gorilla/mux"
	"github.com/justinas/alice"
	"github.com/spf13/viper"
	"github.com/xmidt-org/webpa-common/device"
	"github.com/xmidt-org/webpa-common/logging"
	"github.com/xmidt-org/webpa-common/logging/logginghttp"
	"github.com/xmidt-org/webpa-common/secure"
	"github.com/xmidt-org/webpa-common/secure/handler"
	"github.com/xmidt-org/webpa-common/secure/key"
	"github.com/xmidt-org/webpa-common/service"
	"github.com/xmidt-org/webpa-common/service/servicehttp"
	"github.com/xmidt-org/webpa-common/xhttp"
	"github.com/xmidt-org/webpa-common/xhttp/xcontext"
	"github.com/xmidt-org/webpa-common/xhttp/xfilter"
	"github.com/xmidt-org/webpa-common/xhttp/xtimeout"
	"github.com/xmidt-org/wrp-go/v3/wrphttp"
)

const (
	baseURI = "/api"

	// TODO: Should this change for talaria 2.0?
	version = "v2"

	// TODO: This should be configurable at some point
	poolSize = 1000

	DefaultKeyId = "current"

	DefaultInboundTimeout time.Duration = 120 * time.Second
)

type JWTValidator struct {
	// JWTKeys is used to create the key.Resolver for JWT verification keys
	Keys key.ResolverFactory `json:"keys"`

	// Custom is an optional configuration section that defines
	// custom rules for validation over and above the standard RFC rules.
	Custom secure.JWTValidatorFactory `json:"custom"`
}

func getInboundTimeout(v *viper.Viper) time.Duration {
	if t, err := time.ParseDuration(v.GetString("inbound.timeout")); err == nil {
		return t
	}

	return DefaultInboundTimeout
}

//APIConsumerDeviceAccess contains the properties we will perform checks on for
//any incoming CRUD or statistics device requests
type APIConsumerDeviceAccess struct {
	PartnerIDs []string
}

//let's do this while we figure out how exactly we will be fetching these properties from the
//incoming request
func buildDeviceAccessProperties(_ *http.Request) APIConsumerDeviceAccess {
	return APIConsumerDeviceAccess{
		PartnerIDs: []string{"comcast"},
	}
}

//we need access to the device manage
func prepareDeviceAccessDecorator(logger log.Logger, manager device.Manager) func(http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				//get the device ID
				//fetch the partnerIDs for the device
				//if there is a match, pass to the delegate
				//else return 403
			})
	}
}

func NewPrimaryHandler(logger log.Logger, manager device.Manager, v *viper.Viper, a service.Accessor, e service.Environment, controlConstructor func(http.Handler) http.Handler) (http.Handler, error) {
	var (
		authKeys                     = v.GetStringSlice("inbound.authKey")
		inboundTimeout               = getInboundTimeout(v)
		r                            = mux.NewRouter()
		apiHandler                   = r.PathPrefix(fmt.Sprintf("%s/%s", baseURI, version)).Subrouter()
		authorizationDecorator       = func(h http.Handler) http.Handler { return h }
		authorizationDecoratorDevice = func(h http.Handler) http.Handler { return h }
		deviceAccessDecorator        = prepareDeviceAccessDecorator(logger, manager)
	)

	if len(authKeys) > 0 {
		logger.Log(level.Key(), level.InfoValue(), logging.MessageKey(), "using basic auth", "keyCount", len(authKeys))
		validators := secure.Validators{}
		for _, k := range authKeys {
			validators = append(validators, secure.ExactMatchValidator(k))
		}

		authorizationDecorator = handler.AuthorizationHandler{
			Logger:    logger,
			Validator: validators,
		}.Decorate
	}

	if v.IsSet("jwtValidators") {
		var validator secure.Validator
		var cfgValidators []JWTValidator

		err := v.UnmarshalKey("jwtValidators", &cfgValidators)

		if err != nil {
			return nil, err
		}

		validators := make(secure.Validators, 0, len(cfgValidators))

		for _, validatorDescriptor := range cfgValidators {
			keyResolver, err := validatorDescriptor.Keys.NewResolver()

			if err != nil {
				return nil, fmt.Errorf("Unable to create key resolver: %s", err)
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

		authorizationDecoratorDevice = handler.AuthorizationHandler{
			Logger:    logger,
			Validator: validator,
		}.Decorate
	}

	apiHandler.Handle(
		"/device/send",
		alice.New(
			authorizationDecorator,
			deviceAccessDecorator,
			xtimeout.NewConstructor(xtimeout.Options{
				Timeout: inboundTimeout,
			})).
			Then(wrphttp.NewHTTPHandler(wrpRouterHandler(logger, manager))),
	).Methods("POST", "PATCH")

	apiHandler.Handle("/devices", authorizationDecorator(&device.ListHandler{
		Logger:   logger,
		Registry: manager,
	})).Methods("GET")

	var (
		// the basic decorator chain all device connect handlers use
		deviceConnectChain = alice.New(
			xcontext.Populate(
				logginghttp.SetLogger(
					logger,
					logginghttp.Header(device.DeviceNameHeader, device.DeviceNameHeader),
					logginghttp.RequestInfo,
				),
			),
			controlConstructor,
			device.UseID.FromHeader,
		)

		connectHandler = &device.ConnectHandler{
			Logger:    logger,
			Connector: manager,
		}
	)

	if a != nil && e != nil {
		// if a service discovery environment was configured, append the hash filter to enforce
		// device hashing
		deviceConnectChain.Append(
			xfilter.NewConstructor(
				xfilter.WithFilters(
					servicehttp.NewHashFilter(a, &xhttp.Error{Code: http.StatusGone}, e.IsRegistered),
				),
			),
		)
	}

	// the secured variant of the device connect handler
	apiHandler.Handle(
		"/device",
		deviceConnectChain.Append(authorizationDecoratorDevice).Then(connectHandler),
	).HeadersRegexp("Authorization", ".*")

	// fail open if no authorization header is sent
	apiHandler.Handle(
		"/device",
		deviceConnectChain.Then(connectHandler),
	)

	apiHandler.Handle(
		"/device/{deviceID}/stat",
		alice.New(authorizationDecorator,
			deviceAccessDecorator).Then(&device.StatHandler{
			Logger:   logger,
			Registry: manager,
			Variable: "deviceID",
		}),
	)

	return r, nil
}
