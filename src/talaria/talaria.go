package main

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"time"

	"github.com/Comcast/webpa-common/concurrent"
	"github.com/Comcast/webpa-common/device"
	"github.com/Comcast/webpa-common/device/devicehealth"
	"github.com/Comcast/webpa-common/health"
	"github.com/Comcast/webpa-common/logging"
	"github.com/Comcast/webpa-common/server"
	"github.com/Comcast/webpa-common/service"
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
func startDeviceManagement(logger logging.Logger, h *health.Health, v *viper.Viper) (http.Handler, device.Manager, error) {
	deviceOptions, err := device.NewOptions(logger, v.Sub(device.DeviceManagerKey))
	if err != nil {
		return nil, nil, err
	}

	outbounder, err := NewOutbounder(logger, v.Sub(OutbounderKey))
	if err != nil {
		return nil, nil, err
	}

	outboundListener, err := outbounder.Start()
	if err != nil {
		return nil, nil, err
	}

	connectedDeviceListener, connectedUpdates := (&device.ConnectedDeviceListener{
		RefreshInterval: 10 * time.Second,
	}).Listen()

	deviceOptions.Listeners = []device.Listener{
		connectedDeviceListener,
		outboundListener,
		(&devicehealth.Listener{
			Dispatcher: h,
		}).OnDeviceEvent,
	}

	manager := device.NewManager(deviceOptions, nil)
	primaryHandler, err := NewPrimaryHandler(logger, connectedUpdates, manager, v)
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

		logger, webPA, err = server.Initialize(applicationName, arguments, f, v)
	)

	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to initialize Viper environment: %s\n", err)
		return 1
	}

	logger.Info("Using configuration file: %s", v.ConfigFileUsed())

	//
	// Initialize the manager first, as if it fails we don't want to advertise this service
	//

	health := webPA.Health.NewHealth(logger, devicehealth.Options...)
	primaryHandler, manager, err := startDeviceManagement(logger, health, v)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to start device management: %s\n", err)
		return 1
	}

	_, talariaServer := webPA.Prepare(logger, health, primaryHandler)
	waitGroup, shutdown, err := concurrent.Execute(talariaServer)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to start device manager: %s\n", err)
		return 1
	}

	//
	// Now, initialize the service discovery infrastructure
	//

	serviceOptions, registrar, registeredEndpoints, err := service.Initialize(logger, nil, v.Sub(service.DiscoveryKey))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to initialize service discovery: %s\n", err)
		return 2
	}

	logger.Info("Service options: %#v", serviceOptions)

	var (
		accessor     = service.NewUpdatableAccessor(serviceOptions, nil)
		subscription = service.Subscription{
			Logger:    logger,
			Registrar: registrar,
			Listener: func(endpoints []string) {
				accessor.Update(endpoints)
				manager.DisconnectIf(func(candidate device.ID) bool {
					hashedEndpoint, err := accessor.Get(candidate.Bytes())
					if err != nil {
						logger.Error("Error while attempting to rehash device id %s: %s", candidate, err)
						return true
					}

					// disconnect if hashedEndpoint was NOT found in the endpoints that this talaria instance registered under
					disconnect := !registeredEndpoints.Has(hashedEndpoint)
					if disconnect {
						logger.Info("Disconnecting %s due to service discovery rehash", candidate)
					}

					return disconnect
				})
			},
		}

		signals = make(chan os.Signal, 1)
	)

	if err := subscription.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Unable to run subscription: %s", err)
		return 3
	}

	signal.Notify(signals)
	<-signals
	close(shutdown)
	waitGroup.Wait()

	return 0
}

func main() {
	os.Exit(talaria(os.Args))
}
