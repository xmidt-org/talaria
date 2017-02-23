package main

import (
	"fmt"
	"github.com/Comcast/webpa-common/concurrent"
	"github.com/Comcast/webpa-common/device"
	"github.com/Comcast/webpa-common/server"
	"github.com/Comcast/webpa-common/service"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"routing"
)

const (
	applicationName       = "talaria"
	release               = "Developer"
	defaultVnodeCount int = 211
)

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

	deviceOptions, err := device.NewOptions(logger, v.Sub(device.DeviceManagerKey))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to read device management configuration: %s\n", err)
		return 1
	}

	outbounder, err := routing.NewOutbounder(logger, v.Sub(routing.OutbounderKey))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to read device outbounder configuration: %s\n", err)
		return 1
	}

	deviceOptions.MessageListener = outbounder.NewMessageListener()
	manager := device.NewManager(deviceOptions, nil)
	deviceHandler, err := NewInboundHandler(logger, manager, v)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to initialize inbound handler: %s\n", err)
		return 1
	}

	_, runnable := webPA.Prepare(logger, deviceHandler)
	waitGroup, shutdown, err := concurrent.Execute(runnable)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to start device manager: %s\n", err)
		return 1
	}

	//
	// Now, initialize the service discovery infrastructure
	//

	serviceOptions, registrar, err := service.Initialize(logger, nil, v.Sub(service.DiscoveryKey))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to initialize service discovery: %s\n", err)
		return 2
	}

	logger.Info("Service options: %#v", serviceOptions)

	watch, err := registrar.Watch()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to set watch on services: %s\n", err)
		return 3
	}

	var (
		// TODO: handle rehashes using this subscription
		_       = service.NewAccessorSubscription(watch, nil, serviceOptions)
		signals = make(chan os.Signal, 1)
	)

	signal.Notify(signals)
	<-signals
	close(shutdown)
	waitGroup.Wait()

	return 0
}

func main() {
	os.Exit(talaria(os.Args))
}
