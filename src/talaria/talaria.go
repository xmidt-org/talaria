package main

import (
	"fmt"
	"github.com/Comcast/webpa-common/concurrent"
	"github.com/Comcast/webpa-common/device"
	"github.com/Comcast/webpa-common/server"
	"github.com/Comcast/webpa-common/service"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"net/http"
	_ "net/http/pprof"
	"os"
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
		subscription    = service.NewAccessorSubscription(watch, nil, serviceOptions)
		redirectHandler = service.NewRedirectHandler(
			subscription,
			http.StatusTemporaryRedirect,
			device.IDHashParser(device.DefaultDeviceNameHeader),
			logger,
		)

		_, runnable = webPA.Prepare(logger, redirectHandler)
		signals     = make(chan os.Signal, 1)
	)

	//
	// Execute the runnable, which runs all the servers, and wait for a signal
	//

	if err := concurrent.Await(runnable, signals); err != nil {
		fmt.Fprintf(os.Stderr, "Error when starting %s: %s", applicationName, err)
		return 4
	}

	return 0
}

func main() {
	os.Exit(talaria(os.Args))
}
