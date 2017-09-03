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
func startDeviceManagement(logger log.Logger, h *health.Health, v *viper.Viper) (http.Handler, device.Manager, error) {
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

	var (
		infoLog  = logging.Info(logger)
		errorLog = logging.Error(logger)
	)

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

	serviceOptions, err := service.FromViper(service.Sub(v))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to read service discovery options: %s\n", err)
		return 2
	}

	services, err := service.New(serviceOptions)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to initialize service discovery: %s\n", err)
		return 2
	}

	instancer, err := services.NewInstancer()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to obtain service discovery instancer: %s\n", err)
		return 2
	}

	defer services.Deregister()
	services.Register()
	infoLog.Log("configurationFile", v.ConfigFileUsed(), "serviceOptions", serviceOptions)

	var (
		subscription = service.Subscribe(serviceOptions, instancer)
		signals      = make(chan os.Signal, 1)
	)

	go func() {
		first := true
		for {
			select {
			case u := <-subscription.Updates():
				// throw away the first Accessor, as that is just the initial set of talarias
				if first {
					first = false
					infoLog.Log(logging.MessageKey(), "discarding initial service discovery event")
					continue
				}

				manager.DisconnectIf(func(candidate device.ID) bool {
					instance, err := u.Get(candidate.Bytes())
					if err != nil {
						errorLog.Log(logging.MessageKey(), "Error while attempting to rehash device", "deviceID", candidate, logging.ErrorKey(), err)
						return true
					}

					if instance != serviceOptions.Registration {
						infoLog.Log(logging.MessageKey(), "service discovery rehash", "deviceID", candidate)
						return true
					}

					return false
				})

			case <-subscription.Stopped():
				return
			}
		}
	}()

	signal.Notify(signals)
	<-signals
	close(shutdown)
	waitGroup.Wait()

	return 0
}

func main() {
	os.Exit(talaria(os.Args))
}
