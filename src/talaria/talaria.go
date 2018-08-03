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
	_ "net/http/pprof"
	"os"
	"os/signal"

	"github.com/Comcast/webpa-common/concurrent"
	"github.com/Comcast/webpa-common/device"
	"github.com/Comcast/webpa-common/device/devicehealth"
	"github.com/Comcast/webpa-common/device/rehasher"
	"github.com/Comcast/webpa-common/health"
	"github.com/Comcast/webpa-common/logging"
	"github.com/Comcast/webpa-common/server"
	"github.com/Comcast/webpa-common/service"
	"github.com/Comcast/webpa-common/service/monitor"
	"github.com/Comcast/webpa-common/service/servicecfg"
	"github.com/Comcast/webpa-common/xmetrics"
	"github.com/go-kit/kit/log"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

const (
	applicationName       = "talaria"
	release               = "Developer"
	defaultVnodeCount int = 211
)

// startDeviceManagement handles the configuration and initialization of the device management subsystem
// for talaria.  The returned HTTP handler can be used for device connections and messages, while the returned
// Manager can be used to route and administer the set of connected devices.
func startDeviceManagement(logger log.Logger, h *health.Health, r xmetrics.Registry, v *viper.Viper, c func(http.Handler) http.Handler) (http.Handler, device.Manager, error) {
	deviceOptions, err := device.NewOptions(logger, v.Sub(device.DeviceManagerKey))
	if err != nil {
		return nil, nil, err
	}

	outbounder, err := NewOutbounder(logger, v.Sub(OutbounderKey))
	if err != nil {
		return nil, nil, err
	}

	outboundListener, err := outbounder.Start(NewOutboundMeasures(r))
	if err != nil {
		return nil, nil, err
	}

	deviceOptions.MetricsProvider = r
	deviceOptions.Listeners = []device.Listener{
		outboundListener,
	}

	manager := device.NewManager(deviceOptions)
	primaryHandler, err := NewPrimaryHandler(logger, manager, v, c)
	return primaryHandler, manager, err
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

		logger, metricsRegistry, webPA, err = server.Initialize(applicationName, arguments, f, v, Metrics, device.Metrics, rehasher.Metrics, service.Metrics)
		infoLog                             = logging.Info(logger)
		errorLog                            = logging.Error(logger)
	)

	if err != nil {
		errorLog.Log(logging.MessageKey(), "Unable to initialize Viper environment", logging.ErrorKey(), err)
		return 1
	}

	controlConstructor, err := StartControlServer(logger, v)
	if err != nil {
		errorLog.Log(logging.MessageKey(), "Unable to create control server", logging.ErrorKey(), err)
		return 1
	}

	//
	// Initialize the manager first, as if it fails we don't want to advertise this service
	//

	health := webPA.Health.NewHealth(logger, devicehealth.Options...)
	primaryHandler, manager, err := startDeviceManagement(logger, health, metricsRegistry, v, controlConstructor)
	if err != nil {
		errorLog.Log(logging.MessageKey(), "Unable to start device management", logging.ErrorKey(), err)
		return 2
	}

	_, talariaServer := webPA.Prepare(logger, health, metricsRegistry, primaryHandler)
	waitGroup, shutdown, err := concurrent.Execute(talariaServer)
	if err != nil {
		errorLog.Log(logging.MessageKey(), "Unable to start device manager", logging.ErrorKey(), err)
		return 3
	}

	//
	// Now, initialize the service discovery infrastructure
	//

	e, err := servicecfg.NewEnvironment(logger, v.Sub("service"))
	if err != nil {
		errorLog.Log(logging.MessageKey(), "Unable to initialize service discovery environment", logging.ErrorKey(), err)
		return 4
	}

	if e != nil {
		defer e.Close()
		infoLog.Log("configurationFile", v.ConfigFileUsed())
		e.Register()

		_, err = monitor.New(
			monitor.WithLogger(logger),
			monitor.WithFilter(monitor.NewNormalizeFilter(e.DefaultScheme())),
			monitor.WithEnvironment(e),
			monitor.WithListeners(
				monitor.NewMetricsListener(metricsRegistry),
				monitor.NewRegistrarListener(logger, e, true),

				// this rehasher will handle device disconnects in response to service discovery events
				rehasher.New(
					manager,
					rehasher.WithLogger(logger),
					rehasher.WithEnvironment(e),
					rehasher.WithMetricsProvider(metricsRegistry),
				),
			),
		)

		if err != nil {
			errorLog.Log(logging.MessageKey(), "Unable to start service discovery monitor", logging.ErrorKey(), err)
			return 5
		}
	} else {
		infoLog.Log(logging.MessageKey(), "no service discovery configured")
	}

	signals := make(chan os.Signal, 10)
	signal.Notify(signals)
	s := server.SignalWait(infoLog, signals, os.Interrupt, os.Kill)
	errorLog.Log(logging.MessageKey(), "exiting due to signal", "signal", s)
	close(shutdown)
	waitGroup.Wait()

	return 0
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
