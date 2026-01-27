// SPDX-FileCopyrightText: 2017 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"fmt"
	"io"

	// nolint:gosec
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/xmidt-org/candlelight"
	"github.com/xmidt-org/touchstone"

	// nolint:staticcheck
	"github.com/xmidt-org/webpa-common/v2/xmetrics"

	"github.com/xmidt-org/webpa-common/v2/adapter"
	// nolint:staticcheck
	"github.com/xmidt-org/webpa-common/v2/concurrent"
	"github.com/xmidt-org/webpa-common/v2/device"
	"github.com/xmidt-org/webpa-common/v2/device/devicegate"
	"github.com/xmidt-org/webpa-common/v2/device/devicehealth"
	"github.com/xmidt-org/webpa-common/v2/device/rehasher"

	// nolint:staticcheck
	"github.com/xmidt-org/webpa-common/v2/server"
	// nolint:staticcheck
	"github.com/xmidt-org/webpa-common/v2/service"
	// nolint:staticcheck
	"github.com/xmidt-org/webpa-common/v2/service/accessor"
	"github.com/xmidt-org/webpa-common/v2/service/monitor"

	// nolint:staticcheck
	"github.com/xmidt-org/webpa-common/v2/service/servicecfg"
	"github.com/xmidt-org/webpa-common/v2/xresolver/consul"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gorilla/mux/otelmux"
)

const (
	applicationName  = "talaria"
	tracingConfigKey = "tracing"
	maxDeviceCount   = "max_device_count"
)

var (
	Commit  = "undefined"
	Version = "undefined"
	Date    = "undefined"
)

func setupDefaultConfigValues(v *viper.Viper) {
	v.SetTypeByDefaultValue(true)
	v.SetDefault(RehasherServicesConfigKey, []string{applicationName})
}

func newDeviceManager(logger *zap.Logger, r xmetrics.Registry, tf *touchstone.Factory, v *viper.Viper) (device.Manager, devicegate.Interface, *consul.ConsulWatcher, error) {
	deviceOptions, err := device.NewOptions(logger, v.Sub(device.DeviceManagerKey))
	if err != nil {
		return nil, nil, nil, err
	}

	outbounder, watcher, err := NewOutbounder(logger, v.Sub(OutbounderKey))
	if err != nil {
		return nil, nil, nil, err
	}

	om, err := NewOutboundMeasures(tf)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get OutboundMeasures: %s", err)
	}

	// Create and start Kafka publisher
	kafkaPublisher, err := NewKafkaPublisher(logger, v)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create kafka publisher: %w", err)
	}

	if kafkaPublisher.IsEnabled() {
		if err := kafkaPublisher.Start(); err != nil {
			return nil, nil, nil, fmt.Errorf("failed to start kafka publisher: %w", err)
		}
		logger.Info("Kafka publisher started and enabled")
	}

	outboundListeners, err := outbounder.StartWithKafka(om, kafkaPublisher)
	if err != nil {
		return nil, nil, nil, err
	}

	deviceOptions.MetricsProvider = r
	deviceOptions.Listeners = append(deviceOptions.Listeners, outboundListeners...)

	g := &devicegate.FilterGate{
		FilterStore: make(devicegate.FilterStore),
	}

	deviceOptions.Filter = g
	return device.NewManager(deviceOptions), g, watcher, nil
}

func newStaticMetrics(m device.Manager, r xmetrics.Registry) (err error) {
	r.NewGaugeFunc(maxDeviceCount, func() float64 {
		return float64(m.MaxDevices())
	})
	return
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

		// I hate webpa libary
		logger, metricsRegistry, webPA, err = server.Initialize(applicationName, arguments, f, v, device.Metrics, rehasher.Metrics, service.Metrics)
	)

	// Setup default config values BEFORE server.Initialize reads the config file
	// This ensures AutomaticEnv is enabled and environment variables can override config
	setupDefaultConfigValues(v)

	if parseErr, done := printVersion(f, arguments); done {
		// if we're done, we're exiting no matter what
		if parseErr != nil {
			friendlyError := fmt.Sprintf("failed to parse arguments. detailed error: %s", parseErr)
			logger.Error(friendlyError)
			os.Exit(1)
		}
		os.Exit(0)
	}

	if err != nil {
		logger.Error("unable to initialize Viper environment", zap.Error(err))
		return 1
	}

	tracing, err := loadTracing(v, applicationName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to build tracing component: %v \n", err)
		return 1
	}
	logger.Info("tracing status", zap.Bool("enabled", !tracing.IsNoop()))

	promReg, ok := metricsRegistry.(prometheus.Registerer)
	if !ok {
		fmt.Fprintf(os.Stderr, "failed to get prometheus registerer")

		return 1
	}

	var tsConfig touchstone.Config
	// Get touchstone & zap configurations
	v.UnmarshalKey("touchstone", &tsConfig)
	tf := touchstone.NewFactory(tsConfig, logger, promReg)

	manager, filterGate, watcher, err := newDeviceManager(logger, metricsRegistry, tf, v)
	if err != nil {
		logger.Error("unable to create device manager", zap.Error(err))
		return 2
	}

	err = newStaticMetrics(manager, metricsRegistry)
	if err != nil {
		logger.Error("unable to register static metrics", zap.Error(err))
		return 6
	}

	var log = &adapter.Logger{
		Logger: logger,
	}
	e, err := servicecfg.NewEnvironment(log, v.Sub("service"))
	if err != nil {
		logger.Error("unable to initialize service discovery environment", zap.Error(err))
		return 4
	}

	controlConstructor, err := StartControlServer(logger, manager, filterGate, tf, v, tracing)
	if err != nil {
		logger.Error("unable to create control server", zap.Error(err))
		return 3
	}

	health := webPA.Health.NewHealth(logger, devicehealth.Options...)
	var a *accessor.UpdatableAccessor
	if e != nil {
		// service discovery is optional
		a = new(accessor.UpdatableAccessor)
	}

	rootRouter := mux.NewRouter()
	otelMuxOptions := []otelmux.Option{
		otelmux.WithPropagators(tracing.Propagator()),
		otelmux.WithTracerProvider(tracing.TracerProvider()),
	}
	rootRouter.Use(otelmux.Middleware("primary", otelMuxOptions...), candlelight.EchoFirstTraceNodeInfo(tracing, true))

	primaryHandler, err := NewPrimaryHandler(logger, manager, v, a, e, controlConstructor, tf, rootRouter)
	if err != nil {
		logger.Error("unable to start device management", zap.Error(err))
		return 4
	}

	_, talariaServer, done := webPA.Prepare(logger, health, metricsRegistry, primaryHandler)
	waitGroup, shutdown, err := concurrent.Execute(talariaServer)
	if err != nil {
		logger.Error("unable to start device manager", zap.Error(err))
		return 5
	}

	if e != nil {
		defer e.Close()
		logger.Info("viper successfully retreived confirguation file", zap.String("configurationFile", v.ConfigFileUsed()))
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
			logger.Error("Unable to start service discovery monitor", zap.Error(err))
			return 5
		}
	} else {
		logger.Info("no service discovery configured")
	}

	signals := make(chan os.Signal, 10)
	signal.Notify(signals, syscall.SIGTERM, os.Interrupt)
	for exit := false; !exit; {
		select {
		case s := <-signals:
			logger.Error("exiting due to signal", zap.Any("signal", s))
			exit = true
		case <-done:
			logger.Error("one or more servers exited")
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
	fmt.Fprintf(writer, "  built time: \t%s\n", Date)
	fmt.Fprintf(writer, "  git commit: \t%s\n", Commit)
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
