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
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/goph/emperror"
	"github.com/gorilla/mux"
	"github.com/justinas/alice"
	"github.com/spf13/viper"
	"github.com/xmidt-org/bascule"
	"github.com/xmidt-org/webpa-common/basculechecks"
	"github.com/xmidt-org/webpa-common/xmetrics"

	"github.com/xmidt-org/bascule/basculehttp"
	"github.com/xmidt-org/bascule/key"
	"github.com/xmidt-org/webpa-common/basculechecks"
	"github.com/xmidt-org/webpa-common/device"
	"github.com/xmidt-org/webpa-common/logging"
	"github.com/xmidt-org/webpa-common/logging/logginghttp"
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

	DefaultKeyID = "current"

	DefaultInboundTimeout time.Duration = 120 * time.Second

	apiJWTAttributeKey = "api-jwt"

	// Configuration file dot-delimited paths for convenience and protection against typos

	//JWTValidatorConfigKey is the path to the JWT
	//validator config for device registration endpoints
	JWTValidatorConfigKey = "jwtValidator"

	//DeviceAccessValidatorConfigKey is the path to the validator config for
	//restricting API access to devices based on known device metadata and credentials
	//presented by API consumers
	DeviceAccessValidatorConfigKey = "deviceAccessValidator"

	//ServiceBasicAuthConfigKey is the path to the list of accepted basic auth keys
	//for the API endpoints (note: does not include device registration)
	ServiceBasicAuthConfigKey = "inbound.authKey"

	//InboundTimeoutConfigKey is the path to the request timeout duration for
	//requests inbound to devices connected to talaria
	InboundTimeoutConfigKey = "inbound.timeout"
)

//NoOpConstructor provides a transparent way for constructors that make up
//our middleware chains to work out of the box even without configuration
//such as authentication layers
var NoOpConstructor = func(h http.Handler) http.Handler { return h }

//TODO: should this be provided by bascule since it is a very similar structure?
//across all devices
//JWTValidator provides a convenient way to define jwt validator through config files
type JWTValidator struct {
	// JWTKeys is used to create the key.Resolver for JWT verification keys
	Keys key.ResolverFactory `json:"keys"`

	// Leeway is used to set the amount of time buffer should be given to JWT
	// time values, such as nbf
	Leeway bascule.Leeway
}

func getInboundTimeout(v *viper.Viper) time.Duration {
	if t, err := time.ParseDuration(v.GetString(InboundTimeoutConfigKey)); err == nil {
		return t
	}

	return DefaultInboundTimeout
}

//TODO: See if SetLogger and GetLogger could be provided by bascule since it seems to be
//boilerplate code across services

func SetLogger(logger log.Logger) func(delegate http.Handler) http.Handler {
	return func(delegate http.Handler) http.Handler {
		return http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				r = r.WithContext(logging.WithLogger(r.Context(),
					log.With(logger, "requestHeaders", r.Header, "requestURL", r.URL.EscapedPath(), "method", r.Method)))
				delegate.ServeHTTP(w, r)
			})
	}
}

func GetLogger(ctx context.Context) bascule.Logger {
	logger := log.With(logging.GetLogger(ctx), "ts", log.DefaultTimestampUTC, "caller", log.DefaultCaller)
	return logger
}

//buildUserPassMap decodes base64-encoded strings of the form user:pass and write them to a map from user -> pass
func buildUserPassMap(logger log.Logger, encodedBasicAuthKeys []string) (userPass map[string]string) {
	userPass = make(map[string]string)

	for _, encodedKey := range encodedBasicAuthKeys {
		decoded, err := base64.StdEncoding.DecodeString(encodedKey)
		if err != nil {
			logging.Info(logger).Log(logging.MessageKey(), "Failed to base64-decode basic auth key", "key", encodedKey, logging.ErrorKey(), err)
		}

		i := bytes.IndexByte(decoded, ':')
		logging.Debug(logger).Log(logging.MessageKey(), "Decoded basic auth key", "key", decoded, "delimeterIndex", i)
		if i > 0 {
			userPass[string(decoded[:i])] = string(decoded[i+1:])
		}
	}
	return
}

func NewPrimaryHandler(logger log.Logger, manager device.Manager, v *viper.Viper, a service.Accessor, e service.Environment,
	controlConstructor alice.Constructor, metricsRegistry xmetrics.Registry) (http.Handler, error) {
	var (
		serviceBasicAuthKeys = v.GetStringSlice(ServiceBasicAuthConfigKey)
		inboundTimeout       = getInboundTimeout(v)
		r                    = mux.NewRouter()
		apiHandler           = r.PathPrefix(fmt.Sprintf("%s/%s", baseURI, version)).Subrouter()

		authConstructor = NoOpConstructor
		authEnforcer    = NoOpConstructor

		deviceAuthRules  = bascule.Validators{} //auth rules for device registration endpoints
		serviceAuthRules = bascule.Validators{} //auth rules for everything else

		infoLogger  = logging.Info(logger)
		errorLogger = logging.Error(logger)
		debugLogger = logging.Debug(logger)

		m *basculechecks.JWTValidationMeasures
	)

	if metricsRegistry != nil {
		m = basculechecks.NewJWTValidationMeasures(metricsRegistry)
	}

	listener := basculechecks.NewMetricListener(m)

	authConstructorOptions := []basculehttp.COption{
		basculehttp.WithCLogger(GetLogger),
		basculehttp.WithCErrorResponseFunc(listener.OnErrorResponse),
	}

	if v.IsSet(JWTValidatorConfigKey) {
		var jwtVal JWTValidator
		v.UnmarshalKey(JWTValidatorConfigKey, &jwtVal)

		if jwtVal.Keys.URI != "" {
			resolver, err := jwtVal.Keys.NewResolver()
			if err != nil {
				return nil, emperror.With(err, "Failed to create JWT token key resolver")
			}

			authConstructorOptions = append(authConstructorOptions,
				basculehttp.WithTokenFactory("Bearer",
					basculehttp.BearerTokenFactory{
						DefaultKeyId: DefaultKeyID,
						Resolver:     resolver,
						Parser:       bascule.DefaultJWTParser,
						Leeway:       jwtVal.Leeway,
					}))

			deviceAuthRules = append(deviceAuthRules,
				bascule.Validators{
					bascule.CreateNonEmptyPrincipalCheck(),
					bascule.CreateNonEmptyTypeCheck(),
					bascule.CreateValidTypeCheck([]string{"jwt"}),
				})
		}
	}

	if v.IsSet(DeviceAccessValidatorConfigKey) {
		deviceAccessValidator := &DeviceAccessValidator{}

		if err := v.UnmarshalKey(DeviceAccessValidatorConfigKey, deviceAccessValidator); err != nil {
			errorLogger.Log(logging.MessageKey(), "Could not unmarshall validator config for api access to device.")
			return nil, err
		}

		infoLogger.Log(logging.MessageKey(), "Enabling validator for API access to devices.")

		deviceAccessValidator.manager = manager
		deviceAccessValidator.jwtTokenAttributeKey = apiJWTAttributeKey
		deviceAccessValidator.checkParser = &checkParser{
			pathParser: DefaultPathParser,
		}

		deviceAccessValidator.mapLoader = &simpleMapLoader{}

		if err := deviceAccessValidator.parseChecks(); err != nil {
			errorLogger.Log(logging.ErrorKey(), err, logging.MessageKey(), "Could not parse api access to device validation checks from config")
			return nil, err
		}

		serviceAuthRules = append(serviceAuthRules, deviceAccessValidator)

		userPassMap := buildUserPassMap(logger, serviceBasicAuthKeys)
		debugLogger.Log("BasicAuthUserPassMap", userPassMap)

		if len(userPassMap) > 0 {
			authConstructorOptions = append(authConstructorOptions,
				basculehttp.WithTokenFactory("Basic",
					&AttributedBasicTokenFactory{
						UserPassMap:                userPassMap,
						TargetAttributeKey:         apiJWTAttributeKey,
						VerifiedJWTTokenHeaderName: deviceAccessValidator.APIJWTHeaderName,
					}))

			serviceAuthRules = append(serviceAuthRules, bascule.CreateAllowAllCheck())
		}
	}

	authConstructor = basculehttp.NewConstructor(authConstructorOptions...)

	authEnforcer = basculehttp.NewEnforcer(
		basculehttp.WithELogger(GetLogger),
		basculehttp.WithRules("Basic", serviceAuthRules),
		basculehttp.WithRules("Bearer", deviceAuthRules),
		basculehttp.WithEErrorResponseFunc(listener.OnErrorResponse),
	)

	authChain := alice.New(SetLogger(logger), authConstructor, authEnforcer, basculehttp.NewListenerDecorator(listener))

	apiHandler.Handle("/device/send",
		alice.New(
			device.UseID.FromHeader,
			xtimeout.NewConstructor(xtimeout.Options{
				Timeout: inboundTimeout,
			})).
			Extend(authChain).
			Then(wrphttp.NewHTTPHandler(wrpRouterHandler(logger, manager))),
	).Methods("POST", "PATCH")

	apiHandler.Handle("/devices",
		authChain.Then(&device.ListHandler{
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
		deviceConnectChain.
			Extend(authChain).
			Append(DeviceMetadataMiddleware).
			Then(connectHandler),
	).HeadersRegexp("Authorization", ".*")

	apiHandler.Handle(
		"/device",
		deviceConnectChain.
			Append(DeviceMetadataMiddleware).
			Then(connectHandler),
	)
	apiHandler.Handle(
		"/device/{deviceID}/stat",
		alice.New(
			device.UseID.FromPath("deviceID")).
			Extend(authChain).
			Then(&device.StatHandler{
				Logger:   logger,
				Registry: manager,
				Variable: "deviceID",
			}),
	)

	return r, nil
}
