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
	"io"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"

	"github.com/gorilla/mux"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/xmidt-org/candlelight"
	"github.com/xmidt-org/webpa-common/basculemetrics"
	"github.com/xmidt-org/webpa-common/concurrent"
	"github.com/xmidt-org/webpa-common/device"
	"github.com/xmidt-org/webpa-common/device/devicegate"
	"github.com/xmidt-org/webpa-common/device/devicehealth"
	"github.com/xmidt-org/webpa-common/device/rehasher"
	"github.com/xmidt-org/webpa-common/logging"
	"github.com/xmidt-org/webpa-common/server"
	"github.com/xmidt-org/webpa-common/service"
	"github.com/xmidt-org/webpa-common/service/monitor"
	"github.com/xmidt-org/webpa-common/service/servicecfg"
	"github.com/xmidt-org/webpa-common/xmetrics"
	"github.com/xmidt-org/webpa-common/xresolver/consul"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gorilla/mux/otelmux"
)

const (
	applicationName       = "talaria"
	release               = "Developer"
	defaultVnodeCount int = 211
	tracingConfigKey      = "tracing"
)

var (
	GitCommit = "undefined"
	Version   = "undefined"
	BuildTime = "undefined"
)

func setupDefaultConfigValues(v *viper.Viper) {
	v.SetTypeByDefaultValue(true)
	v.SetDefault(RehasherServicesConfigKey, []string{applicationName})
}

func newDeviceManager(logger log.Logger, r xmetrics.Registry, v *viper.Viper) (device.Manager, devicegate.Interface, *consul.ConsulWatcher, error) {
	deviceOptions, err := device.NewOptions(logger, v.Sub(device.DeviceManagerKey))
	if err != nil {
		return nil, nil, nil, err
	}

	outbounder, watcher, err := NewOutbounder(logger, v.Sub(OutbounderKey))
	if err != nil {
		return nil, nil, nil, err
	}

	outboundListener, err := outbounder.Start(NewOutboundMeasures(r))
	if err != nil {
		return nil, nil, nil, err
	}

	deviceOptions.MetricsProvider = r
	deviceOptions.Listeners = []device.Listener{
		outboundListener,
	}

	g := &devicegate.FilterGate{
		FilterStore: make(devicegate.FilterStore),
	}

	deviceOptions.Filter = g

	return device.NewManager(deviceOptions), g, watcher, nil

}

func loadTracing(v *viper.Viper, appName string) (candlelight.Tracing, error) {
	var traceConfig candlelight.Config
	err := v.UnmarshalKey(tracingConfigKey, &traceConfig)
	if err != nil {
		return candlelight.Tracing{}, err
	}
	traceConfig.ApplicationName = appName

	tracing, err := candlelight.New(traceConfig)
	if err != nil {
		return candlelight.Tracing{}, err
	}

	return tracing, nil
}

// talaria is the driver function for Talaria.  It performs everything main() would do,
// except for obtaining the command-line arguments (which are passed to it).
func talaria(arguments []string) int {
	//
	// Initialize the server environment: command-line flags, Viper, logging, and the WebPA instance
	//

	var (
		f = pflag.NewFlagSet(applicationName, pflag.ContinueOnError)
		v = viper.New()

		logger, metricsRegistry, webPA, err = server.Initialize(applicationName, arguments, f, v, Metrics, device.Metrics, rehasher.Metrics, service.Metrics, basculemetrics.Metrics)
		infoLogger                          = level.Info(logger)
	)

	if parseErr, done := printVersion(f, arguments); done {
		// if we're done, we're exiting no matter what
		if parseErr != nil {
			friendlyError := fmt.Sprintf("failed to parse arguments. detailed error: %s", parseErr)
			logging.Error(logger).Log(
				logging.ErrorKey(),
				friendlyError)
			os.Exit(1)
		}
		os.Exit(0)
	}

	setupDefaultConfigValues(v)

	if err != nil {
		logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), "Unable to initialize Viper environment", logging.ErrorKey(), err)
		return 1
	}

	tracing, err := loadTracing(v, applicationName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to build tracing component: %v \n", err)
		return 1
	}
	infoLogger.Log(logging.MessageKey(), "tracing status", "enabled", !tracing.IsNoop())

	manager, filterGate, watcher, err := newDeviceManager(logger, metricsRegistry, v)
	if err != nil {
		logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), "Unable to create device manager", logging.ErrorKey(), err)
		return 2
	}

	e, err := servicecfg.NewEnvironment(logger, v.Sub("service"))
	if err != nil {
		logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), "Unable to initialize service discovery environment", logging.ErrorKey(), err)
		return 4
	}

	controlConstructor, err := StartControlServer(logger, manager, filterGate, metricsRegistry, v, tracing)
	if err != nil {
		logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), "Unable to create control server", logging.ErrorKey(), err)
		return 3
	}

	health := webPA.Health.NewHealth(logger, devicehealth.Options...)
	var a *service.UpdatableAccessor
	if e != nil {
		// service discovery is optional
		a = new(service.UpdatableAccessor)
	}

	rootRouter := mux.NewRouter()
	otelMuxOptions := []otelmux.Option{
		otelmux.WithPropagators(tracing.Propagator()),
		otelmux.WithTracerProvider(tracing.TracerProvider()),
	}
	rootRouter.Use(otelmux.Middleware("primary", otelMuxOptions...), candlelight.EchoFirstTraceNodeInfo(tracing.Propagator()))

	primaryHandler, err := NewPrimaryHandler(logger, manager, v, a, e, controlConstructor, metricsRegistry, rootRouter)
	if err != nil {
		logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), "Unable to start device management", logging.ErrorKey(), err)
		return 4
	}

	_, talariaServer, done := webPA.Prepare(logger, health, metricsRegistry, primaryHandler)
	waitGroup, shutdown, err := concurrent.Execute(talariaServer)
	if err != nil {
		logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), "Unable to start device manager", logging.ErrorKey(), err)
		return 5
	}

	if e != nil {
		defer e.Close()
		logger.Log(level.Key(), level.InfoValue(), "configurationFile", v.ConfigFileUsed())
		e.Register()

		listeners := []monitor.Listener{
			monitor.NewMetricsListener(metricsRegistry),
			monitor.NewRegistrarListener(logger, e, true),
			monitor.NewAccessorListener(e.AccessorFactory(), a.Update),
			// this rehasher will handle device disconnects in response to service discovery events
			rehasher.New(
				manager,
				v.GetStringSlice(RehasherServicesConfigKey),
				rehasher.WithLogger(logger),
				rehasher.WithIsRegistered(e.IsRegistered),
				rehasher.WithMetricsProvider(metricsRegistry),
			),
		}
		if watcher != nil {
			listeners = append(listeners, watcher)
		}

		_, err = monitor.New(
			monitor.WithLogger(logger),
			monitor.WithFilter(monitor.NewNormalizeFilter(e.DefaultScheme())),
			monitor.WithEnvironment(e),
			monitor.WithListeners(listeners...),
		)

		if err != nil {
			logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), "Unable to start service discovery monitor", logging.ErrorKey(), err)
			return 5
		}
	} else {
		logger.Log(level.Key(), level.InfoValue(), logging.MessageKey(), "no service discovery configured")
	}

	signals := make(chan os.Signal, 10)
	signal.Notify(signals, os.Kill, os.Interrupt)
	for exit := false; !exit; {
		select {
		case s := <-signals:
			logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), "exiting due to signal", "signal", s)
			exit = true
		case <-done:
			logger.Log(level.Key(), level.ErrorValue(), logging.MessageKey(), "one or more servers exited")
			exit = true
		}
	}

	close(shutdown)
	waitGroup.Wait()
	return 0
}

func printVersion(f *pflag.FlagSet, arguments []string) (error, bool) {
	printVer := f.BoolP("version", "v", false, "displays the version number")
	if err := f.Parse(arguments); err != nil {
		return err, true
	}

	if *printVer {
		printVersionInfo(os.Stdout)
		return nil, true
	}
	return nil, false
}

func printVersionInfo(writer io.Writer) {
	fmt.Fprintf(writer, "%s:\n", applicationName)
	fmt.Fprintf(writer, "  version: \t%s\n", Version)
	fmt.Fprintf(writer, "  go version: \t%s\n", runtime.Version())
	fmt.Fprintf(writer, "  built time: \t%s\n", BuildTime)
	fmt.Fprintf(writer, "  git commit: \t%s\n", GitCommit)
	fmt.Fprintf(writer, "  os/arch: \t%s/%s\n", runtime.GOOS, runtime.GOARCH)
}

func main() {
	os.Exit(
		func() int {
			result := talaria(os.Args)
			fmt.Printf("exiting with code: %d\n", result)
			return result
		}(),
	)
}
