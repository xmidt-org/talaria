// SPDX-FileCopyrightText: 2023 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io/fs"
	"os"
	"time"

	"github.com/goschtalt/goschtalt"
	"github.com/xmidt-org/arrange/arrangehttp"
	"github.com/xmidt-org/retry"
	"github.com/xmidt-org/sallust"
	"github.com/xmidt-org/wrp-go/v3"
	"gopkg.in/dealancer/validate.v2"
)

// Config is the configuration for the xmidt-agent.
type Config struct {
	Client           Client
	Identity         Identity
	OperationalState OperationalState
	XmidtCredentials XmidtCredentials
	XmidtService     XmidtService
	Logger           sallust.Config
	Storage          Storage
}

type Client struct {
	//  FetchURL is the url used to fetch the WS url.
	FetchURL string
	//  (optional) FetchURLTimeout is the timeout for the fetching the WS url. If this is not set, the default is 30 seconds.
	FetchURLTimeout time.Duration
	//  (optional) PingInterval is the ping interval allowed for the WS connection.
	PingInterval time.Duration
	//  (optional) PingTimeout is the ping timeout for the WS connection.
	PingTimeout time.Duration
	//  (optional) ConnectTimeout is the connect timeout for the WS connection.
	ConnectTimeout time.Duration
	//  (optional) KeepAliveInterval is the keep alive interval for the WS connection.
	KeepAliveInterval time.Duration
	//  (optional) IdleConnTimeout is the idle connection timeout for the WS connection.
	IdleConnTimeout time.Duration
	//  (optional) TLSHandshakeTimeout is the TLS handshake timeout for the WS connection.
	TLSHandshakeTimeout time.Duration
	//  (optional) ExpectContinueTimeout is the expect continue timeout for the WS connection.
	ExpectContinueTimeout time.Duration
	//  (optional) MaxMessageBytes is the largest allowable message to send or receive.
	MaxMessageBytes int64
	// (optional) DisableV4 determines whether or not to allow IPv4 for the WS connection.
	// If this is not set, the default is false (IPv4 is enabled).
	// Either V4 or V6 can be disabled, but not both.
	DisableV4 bool
	// (optional) DisableV6 determines whether or not to allow IPv6 for the WS connection.
	// If this is not set, the default is false (IPv6 is enabled).
	// Either V4 or V6 can be disabled, but not both.
	DisableV6 bool
	//  (optional) RetryPolicy sets the retry policy factory used for delaying between retry attempts for reconnection.
	RetryPolicy retry.Config
	// (optional) Once sets whether or not to only attempt to connect once.
	Once bool
}

// Identity contains the information that identifies the device.
type Identity struct {
	// DeviceID is the unique identifier for the device.  Generally this is a
	// MAC address of the "primary" network interface.
	DeviceID wrp.DeviceID

	// SerialNumber is the serial number of the device.
	SerialNumber string

	// Manufacturer is the name of the manufacturer of the device.
	HardwareModel string

	// HardwareManufacturer is the name of the manufacturer of the hardware.
	HardwareManufacturer string

	// FirmwareVersion is the version of the firmware.
	FirmwareVersion string

	// PartnerID is the identifier for the partner that the device is associated
	PartnerID string
}

// OperationalState contains the information about the device's operational state.
type OperationalState struct {
	// LastRebootReason is the reason for the last reboot.
	LastRebootReason string

	// BootTime is the time the device was last booted.
	BootTime time.Time
}

// XmidtCredentials contains the information needed to retrieve the credentials
// from the XMiDT credential server.
type XmidtCredentials struct {
	// URL is the URL of the XMiDT credential server.
	URL string

	// HTTPClient is the configuration for the HTTP client used to retrieve the
	// credentials.
	HTTPClient arrangehttp.ClientConfig

	// RefetchPercent is the percentage of the time between the last fetch and
	// the expiration time to refetch the credentials.  For example, if the
	// credentials are valid for 1 hour and the refetch percent is 90, then the
	// credentials will be refetched after 54 minutes.
	RefetchPercent float64

	// FileName is the name and path of the file to store the credentials.  There
	// will be another file with the same name and a ".sha256" extension that
	// contains the SHA256 hash of the credentials file.
	FileName string

	// FilePermissions is the permissions to use when creating the credentials
	// file.
	FilePermissions fs.FileMode
}

// XmidtService contains the configuration for the XMiDT service endpoint.
type XmidtService struct {
	// URL is the URL of the XMiDT service endpoint.  This is the endpoint that
	// the device will connect to or use as the fqdn for the JWT TXT redirector.
	URL string

	// Backoff is the parameters that limit the retry backoff algorithm.
	Backoff Backoff

	// JwtTxtRedirector is the configuration for the JWT TXT redirector.  If left
	// empty the JWT TXT redirector is disabled.
	JwtTxtRedirector JwtTxtRedirector
}

// JwtTxtRedirector contains the configuration for the JWT TXT redirector.
type JwtTxtRedirector struct {
	// AllowedAlgorithms is the list of allowed algorithms for the JWT TXT
	// redirector. Only specified algorithms will be used for verification.
	// Valid values are:
	//	- "EdDSA"
	//	- "ES256", "ES384", "ES512"
	//	- "PS256", "PS384", "PS512"
	//	- "RS256", "RS384", "RS512"
	AllowedAlgorithms []string

	// Timeout is the timeout for the JWT TXT redirector request.
	Timeout time.Duration

	// PEMs is the list of PEM-encoded public keys to use for verification.
	PEMs []string

	// PEMFiles is the list of files containing PEM-encoded public keys to use
	PEMFiles []string
}

// Backoff defines the parameters that limit the retry backoff algorithm.
// The retries are a geometric progression.
// 1, 3, 7, 15, 31 ... n = (2n+1)
type Backoff struct {
	MinDelay time.Duration
	MaxDelay time.Duration
}

type Storage struct {
	Temporary string
	Durable   string
}

// Collect and process the configuration files and env vars and
// produce a configuration object.
func provideConfig(cli *CLI) (*goschtalt.Config, error) {
	gs, err := goschtalt.New(
		goschtalt.StdCfgLayout(applicationName, cli.Files...),
		goschtalt.ConfigIs("two_words"),
		goschtalt.DefaultUnmarshalOptions(
			goschtalt.WithValidator(
				goschtalt.ValidatorFunc(validate.Validate),
			),
		),

		// Seed the program with the default, built-in configuration.
		// Mark this as a default so it is ordered correctly.
		goschtalt.AddValue("built-in", goschtalt.Root, defaultConfig,
			goschtalt.AsDefault()),
	)
	if err != nil {
		return nil, err
	}

	if cli.Show {
		// handleCLIShow handles the -s/--show option where the configuration is
		// shown, then the program is exited.
		//
		// Exit with success because if the configuration is broken it will be
		// very hard to debug where the problem originates.  This way you can
		// see the configuration and then run the service with the same
		// configuration to see the error.

		fmt.Fprintln(os.Stdout, gs.Explain().String())

		out, err := gs.Marshal()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
		} else {
			fmt.Fprintln(os.Stdout, "## Final Configuration\n---\n"+string(out))
		}

		os.Exit(0)
	}

	var tmp Config
	err = gs.Unmarshal(goschtalt.Root, &tmp)
	if err != nil {
		fmt.Fprintln(os.Stderr, "There is a critical error in the configuration.")
		fmt.Fprintln(os.Stderr, "Run with -s/--show to see the configuration.")
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)

		// Exit here to prevent a very difficult to debug error from occurring.
		os.Exit(0)
	}

	return gs, nil
}

// -----------------------------------------------------------------------------
// Keep the default configuration at the bottom of the file so it is easy to
// see what the default configuration is.
// -----------------------------------------------------------------------------

var defaultConfig = Config{
	XmidtCredentials: XmidtCredentials{
		RefetchPercent:  90.0,
		FileName:        "credentials.msgpack",
		FilePermissions: fs.FileMode(0600),
	},
	XmidtService: XmidtService{
		JwtTxtRedirector: JwtTxtRedirector{
			Timeout: 10 * time.Second,
			AllowedAlgorithms: []string{
				"EdDSA",
				"ES256", "ES384", "ES512",
				"PS256", "PS384", "PS512",
				"RS256", "RS384", "RS512",
			},
		},
	},
}
