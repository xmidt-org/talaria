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
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-kit/kit/metrics"
	"github.com/go-kit/log"
	"github.com/gorilla/mux"
	"github.com/justinas/alice"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/viper"
	"github.com/xmidt-org/bascule"
	"github.com/xmidt-org/clortho/clorthometrics"
	"github.com/xmidt-org/clortho/clorthozap"
	"github.com/xmidt-org/sallust"
	"github.com/xmidt-org/touchstone"

	// nolint:staticcheck
	"github.com/xmidt-org/webpa-common/v2/xmetrics"
	"go.uber.org/zap"

	// nolint:staticcheck
	"github.com/xmidt-org/bascule/basculechecks"
	// nolint:staticcheck
	"github.com/xmidt-org/bascule/basculehttp"
	"github.com/xmidt-org/clortho"

	// nolint:staticcheck
	"github.com/xmidt-org/webpa-common/v2/basculemetrics"
	"github.com/xmidt-org/webpa-common/v2/device"

	// nolint:staticcheck
	"github.com/xmidt-org/webpa-common/v2/logging"
	// nolint:staticcheck
	"github.com/xmidt-org/webpa-common/v2/logging/logginghttp"
	// nolint:staticcheck
	"github.com/xmidt-org/webpa-common/v2/service"
	// nolint:staticcheck
	"github.com/xmidt-org/webpa-common/v2/service/servicehttp"
	"github.com/xmidt-org/webpa-common/v2/xhttp"
	"github.com/xmidt-org/webpa-common/v2/xhttp/xcontext"
	"github.com/xmidt-org/webpa-common/v2/xhttp/xfilter"
	"github.com/xmidt-org/webpa-common/v2/xhttp/xtimeout"
	"github.com/xmidt-org/wrp-go/v3/wrphttp"
)

const (
	baseURI = "/api"

	// TODO: Should this change for talaria 2.0?
	version = "v3"
	v2      = "v2"

	DefaultKeyID = "current"

	DefaultInboundTimeout time.Duration = 120 * time.Second
)

// Paths to configuration values for convenience and protection against typos.
const (
	// JWTValidatorConfigKey is the path to the JWT
	// validator config for device registration endpoints.
	JWTValidatorConfigKey = "jwtValidator"

	// DeviceAccessCheckConfigKey is the path to the validator config for
	// restricting API access to devices based on known device metadata and credentials
	// presented by API consumers.
	DeviceAccessCheckConfigKey = "deviceAccessCheck"

	// ServiceBasicAuthConfigKey is the path to the list of accepted basic auth keys
	// for the API endpoints (note: does not include device registration).
	ServiceBasicAuthConfigKey = "inbound.authKey"

	// InboundTimeoutConfigKey is the path to the request timeout duration for
	// requests inbound to devices connected to talaria.
	InboundTimeoutConfigKey = "inbound.timeout"

	// RehasherServicesConfigKey is the path to the services for whose events talaria's
	// rehasher should listen to.
	RehasherServicesConfigKey = "device.rehasher.services"
)

// NoOpConstructor provides a transparent way for constructors that make up
// our middleware chains to work out of the box even without configuration
// such as authentication layers
var NoOpConstructor = func(h http.Handler) http.Handler { return h }

// JWTValidator provides a convenient way to define jwt validator through config files
type JWTValidator struct {
	// Config is used to create the clortho Resolver for JWT verification keys
	Config clortho.Config `json:"config"`

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

// buildUserPassMap decodes base64-encoded strings of the form user:pass and write them to a map from user -> pass
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
	controlConstructor alice.Constructor, metricsRegistry xmetrics.Registry, r *mux.Router) (http.Handler, error) {
	var (
		inboundTimeout = getInboundTimeout(v)
		apiHandler     = r.PathPrefix(fmt.Sprintf("%s/{version:%s|%s}", baseURI, v2, version)).Subrouter()

		authConstructor = NoOpConstructor
		authEnforcer    = NoOpConstructor

		deviceAuthRules  = bascule.Validators{} //auth rules for device registration endpoints
		serviceAuthRules = bascule.Validators{} //auth rules for everything else

		infoLogger  = logging.Info(logger)
		errorLogger = logging.Error(logger)

		m        = basculemetrics.NewAuthValidationMeasures(metricsRegistry)
		listener = basculemetrics.NewMetricListener(m)
	)

	authConstructorOptions := []basculehttp.COption{
		basculehttp.WithCLogger(getLogger),
		basculehttp.WithCErrorResponseFunc(listener.OnErrorResponse),
	}

	if v.IsSet(JWTValidatorConfigKey) {
		var jwtVal JWTValidator
		v.UnmarshalKey(JWTValidatorConfigKey, &jwtVal)

		kr := clortho.NewKeyRing()

		// Instantiate a fetcher for the resolver
		f, err := clortho.NewFetcher()
		if err != nil {
			return nil, errors.New("failed to create clortho fetcher")
		}

		resolver, err := clortho.NewResolver(
			clortho.WithConfig(jwtVal.Config),
			clortho.WithKeyRing(kr),
			clortho.WithFetcher(f),
		)
		if err != nil {
			return nil, errors.New("failed to create clortho reolver")
		}

		promReg, ok := metricsRegistry.(prometheus.Registerer)
		if !ok {
			return nil, errors.New("failed to get prometheus registerer")

		}

		var (
			tsConfig touchstone.Config
			zConfig  sallust.Config
		)
		// Get touchstone & zap configurations
		v.UnmarshalKey("touchstone", &tsConfig)
		v.UnmarshalKey("zap", &zConfig)
		zlogger := zap.Must(zConfig.Build())
		tf := touchstone.NewFactory(tsConfig, zlogger, promReg)
		// Instantiate a metric listener for the resolver
		cml, err := clorthometrics.NewListener(clorthometrics.WithFactory(tf))
		if err != nil {
			return nil, errors.New("failed to create clortho metrics listener")

		}

		// Instantiate a logging listener for the resolver
		czl, err := clorthozap.NewListener(
			clorthozap.WithLogger(zlogger),
		)
		if err != nil {
			return nil, errors.New("failed to create clortho zap logger listener")

		}

		resolver.AddListener(cml)
		resolver.AddListener(czl)

		authConstructorOptions = append(authConstructorOptions, basculehttp.WithTokenFactory("Bearer", basculehttp.BearerTokenFactory{
			DefaultKeyID: DefaultKeyID,
			Resolver:     resolver,
			Parser:       bascule.DefaultJWTParser,
			Leeway:       jwtVal.Leeway,
		}))

		deviceAuthRules = append(deviceAuthRules,
			bascule.Validators{
				basculechecks.NonEmptyPrincipal(),
				basculechecks.NonEmptyType(),
				basculechecks.ValidType([]string{"jwt"}),
			})
	}

	if v.IsSet(ServiceBasicAuthConfigKey) {
		userPassMap := buildUserPassMap(logger, v.GetStringSlice(ServiceBasicAuthConfigKey))

		if len(userPassMap) > 0 {
			authConstructorOptions = append(authConstructorOptions,
				basculehttp.WithTokenFactory("Basic", basculehttp.BasicTokenFactory(userPassMap)))

			serviceAuthRules = append(serviceAuthRules, basculechecks.AllowAll())
		}
	}

	wrpRouterHandler := wrpRouterHandler(logger, manager, getLogger)

	if v.IsSet(DeviceAccessCheckConfigKey) {
		config := new(deviceAccessCheckConfig)

		if err := v.UnmarshalKey(DeviceAccessCheckConfigKey, config); err != nil {
			errorLogger.Log(logging.MessageKey(), "Could not unmarshall wrpCheck config for api access to device.")
			return nil, err
		}

		deviceAccessCheck, err := buildDeviceAccessCheck(config, logger, metricsRegistry.NewCounter(InboundWRPMessageCounter), manager)
		if err != nil {
			return nil, err
		}

		infoLogger.Log(logging.MessageKey(), "Enabling Device Access Validator.")
		wrpRouterHandler = withDeviceAccessCheck(errorLogger, wrpRouterHandler, deviceAccessCheck)
	}

	authConstructor = basculehttp.NewConstructor(authConstructorOptions...)
	authConstructorLegacy := basculehttp.NewConstructor(append([]basculehttp.COption{
		basculehttp.WithCErrorHTTPResponseFunc(basculehttp.LegacyOnErrorHTTPResponse),
	}, authConstructorOptions...)...)

	authEnforcer = basculehttp.NewEnforcer(
		basculehttp.WithELogger(getLogger),
		basculehttp.WithRules("Basic", serviceAuthRules),
		basculehttp.WithRules("Bearer", deviceAuthRules),
		basculehttp.WithEErrorResponseFunc(listener.OnErrorResponse),
	)

	authChain := alice.New(setLogger(logger), authConstructor, authEnforcer, basculehttp.NewListenerDecorator(listener))
	authChainV2 := alice.New(setLogger(logger), authConstructorLegacy, authEnforcer, basculehttp.NewListenerDecorator(listener))

	versionCompatibleAuth := alice.New(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(r http.ResponseWriter, req *http.Request) {
			vars := mux.Vars(req)
			if vars != nil {
				if vars["version"] == v2 {
					authChainV2.Then(next).ServeHTTP(r, req)
					return
				}
			}
			authChain.Then(next).ServeHTTP(r, req)
		})
	})

	apiHandler.Handle("/device/send",
		alice.New(
			xtimeout.NewConstructor(xtimeout.Options{
				Timeout: inboundTimeout,
			})).
			Extend(versionCompatibleAuth).
			Then(wrphttp.NewHTTPHandler(wrpRouterHandler)),
	).Methods("POST", "PATCH")

	apiHandler.Handle("/devices",
		versionCompatibleAuth.Then(&device.ListHandler{
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

	// the secured variant of the device connect handler - compatible with v2 and v3
	r.Handle(
		fmt.Sprintf("%s/{version:%s|%s}/device", baseURI, v2, version),
		deviceConnectChain.
			Extend(versionCompatibleAuth).
			Append(DeviceMetadataMiddleware(getLogger)).
			Then(connectHandler),
	)

	apiHandler.Handle(
		"/device/{deviceID}/stat",
		alice.New(
			device.UseID.FromPath("deviceID")).
			Extend(versionCompatibleAuth).
			Then(&device.StatHandler{
				Logger:   logger,
				Registry: manager,
				Variable: "deviceID",
			}),
	).Methods("GET")

	return r, nil
}

func buildDeviceAccessCheck(config *deviceAccessCheckConfig, logger log.Logger, counter metrics.Counter, deviceRegistry device.Registry) (deviceAccess, error) {
	errorLogger := logging.Error(logger)

	if len(config.Checks) < 1 {
		errorLogger.Log(logging.MessageKey(), "Potential security misconfig. Include checks for deviceAccessCheck or disable it")
		return nil, errors.New("failed enabling DeviceAccessCheck")
	}

	if config.Type != "enforce" && config.Type != "monitor" {
		errorLogger.Log(logging.MessageKey(), "Unexpected type for deviceAccessCheck. Supported types are 'monitor' and 'enforce'")
		return nil, errors.New("failed verifying DeviceAccessCheck type")
	}

	// nolint:prealloc
	var parsedChecks []*parsedCheck
	for _, check := range config.Checks {
		parsedCheck, err := parseDeviceAccessCheck(check)
		if err != nil {
			errorLogger.Log(logging.ErrorKey(), err, logging.MessageKey(), "deviceAccesscheck parse failure")
			return nil, errors.New("failed parsing DeviceAccessCheck checks")
		}
		parsedChecks = append(parsedChecks, parsedCheck)
	}

	if config.Sep == "" {
		config.Sep = "."
	}

	return &talariaDeviceAccess{
		strict:             config.Type == "enforce",
		wrpMessagesCounter: counter,
		checks:             parsedChecks,
		deviceRegistry:     deviceRegistry,
		debugLogger:        logging.Debug(logger),
		sep:                config.Sep,
	}, nil
}
