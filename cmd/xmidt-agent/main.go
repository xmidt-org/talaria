// SPDX-FileCopyrightText: 2023 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"

	"github.com/alecthomas/kong"
	"github.com/goschtalt/goschtalt"
	_ "github.com/goschtalt/goschtalt/pkg/typical"
	_ "github.com/goschtalt/yaml-decoder"
	_ "github.com/goschtalt/yaml-encoder"
	"github.com/xmidt-org/sallust"
	"github.com/xmidt-org/wrp-go/v3"
	"github.com/xmidt-org/xmidt-agent/internal/credentials"
	"github.com/xmidt-org/xmidt-agent/internal/jwtxt"
	"github.com/xmidt-org/xmidt-agent/internal/websocket"
	"github.com/xmidt-org/xmidt-agent/internal/websocket/event"

	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	applicationName = "xmidt-agent"
)

// These match what goreleaser provides.
var (
	commit  = "undefined"
	version = "undefined"
	date    = "undefined"
	builtBy = "undefined"
)

// CLI is the structure that is used to capture the command line arguments.
type CLI struct {
	Dev   bool     `optional:"" short:"d" help:"Run in development mode."`
	Show  bool     `optional:"" short:"s" help:"Show the configuration and exit."`
	Graph string   `optional:"" short:"g" help:"Output the dependency graph to the specified file."`
	Files []string `optional:"" short:"f" help:"Specific configuration files or directories."`
}

type LifeCycleIn struct {
	fx.In
	Logger     *zap.Logger
	LC         fx.Lifecycle
	Shutdowner fx.Shutdowner
	WS         *websocket.Websocket
	CancelList []event.CancelFunc
}

// xmidtAgent is the main entry point for the program.  It is responsible for
// setting up the dependency injection framework and returning the app object.
func xmidtAgent(args []string) (*fx.App, error) {
	var (
		gscfg *goschtalt.Config

		// Capture the dependency tree in case we need to debug something.
		g fx.DotGraph

		// Capture the command line arguments.
		cli *CLI
	)

	app := fx.New(
		fx.Supply(cliArgs(args)),
		fx.Populate(&g),
		fx.Populate(&gscfg),
		fx.Populate(&cli),

		fx.WithLogger(func(log *zap.Logger) fxevent.Logger {
			return &fxevent.ZapLogger{Logger: log}
		}),

		fx.Provide(
			provideCLI,
			provideLogger,
			provideConfig,
			provideCredentials,
			provideInstructions,
			provideWS,
			func(id Identity) wrp.DeviceID {
				return id.DeviceID
			},

			goschtalt.UnmarshalFunc[sallust.Config]("logger", goschtalt.Optional()),
			goschtalt.UnmarshalFunc[Identity]("identity"),
			goschtalt.UnmarshalFunc[OperationalState]("operational_state"),
			goschtalt.UnmarshalFunc[XmidtCredentials]("xmidt_credentials"),
			goschtalt.UnmarshalFunc[XmidtService]("xmidt_service"),
			goschtalt.UnmarshalFunc[Storage]("storage"),
			goschtalt.UnmarshalFunc[Client]("client"),
		),

		fsProvide(),

		fx.Invoke(
			// TODO: Remove this.
			// For now require the credentials to be fetched this way.  Later
			// Other services will depend on this.
			func(*credentials.Credentials) {},

			// TODO: Remove this, too.
			func(i *jwtxt.Instructions) {
				if i != nil {
					s, _ := i.Endpoint(context.Background())
					fmt.Println(s)
				}
			},
			lifeCycle,
		),
	)

	if cli != nil && cli.Graph != "" {
		_ = os.WriteFile(cli.Graph, []byte(g), 0600)
	}

	if err := app.Err(); err != nil {
		return nil, err
	}

	return app, nil
}

func main() {
	app, err := xmidtAgent(os.Args[1:])
	if err == nil {
		app.Run()
		return
	}

	fmt.Fprintln(os.Stderr, err)
	os.Exit(-1)
}

// Provides a named type so it's a bit easier to flow through & use in fx.
type cliArgs []string

// Handle the CLI processing and return the processed input.
func provideCLI(args cliArgs) (*CLI, error) {
	return provideCLIWithOpts(args, false)
}

func provideCLIWithOpts(args cliArgs, testOpts bool) (*CLI, error) {
	var cli CLI

	// Create a no-op option to satisfy the kong.New() call.
	var opt kong.Option = kong.OptionFunc(
		func(*kong.Kong) error {
			return nil
		},
	)

	if testOpts {
		opt = kong.Writers(nil, nil)
	}

	parser, err := kong.New(&cli,
		kong.Name(applicationName),
		kong.Description("The cpe agent for Xmidt service.\n"+
			fmt.Sprintf("\tVersion:  %s\n", version)+
			fmt.Sprintf("\tDate:     %s\n", date)+
			fmt.Sprintf("\tCommit:   %s\n", commit)+
			fmt.Sprintf("\tBuilt By: %s\n", builtBy),
		),
		kong.UsageOnError(),
		opt,
	)
	if err != nil {
		return nil, err
	}

	if testOpts {
		parser.Exit = func(_ int) { panic("exit") }
	}

	_, err = parser.Parse(args)
	if err != nil {
		parser.FatalIfErrorf(err)
	}

	return &cli, nil
}

type LoggerIn struct {
	fx.In
	CLI *CLI
	Cfg sallust.Config
}

// Create the logger and configure it based on if the program is in
// debug mode or normal mode.
func provideLogger(in LoggerIn) (*zap.Logger, error) {
	in.Cfg.EncoderConfig = sallust.EncoderConfig{
		TimeKey:        "T",
		LevelKey:       "L",
		NameKey:        "N",
		CallerKey:      "C",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "M",
		StacktraceKey:  "S",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    "capitalColor",
		EncodeTime:     "RFC3339",
		EncodeDuration: "string",
		EncodeCaller:   "short",
	}

	if in.CLI.Dev {
		in.Cfg.Level = "DEBUG"
		in.Cfg.Development = true
		in.Cfg.Encoding = "console"
		in.Cfg.OutputPaths = append(in.Cfg.OutputPaths, "stderr")
		in.Cfg.ErrorOutputPaths = append(in.Cfg.ErrorOutputPaths, "stderr")
	}

	return in.Cfg.Build()
}

func lifeCycle(in LifeCycleIn) {
	logger := in.Logger.With(zap.String("component", "fx_lifecycle"))
	in.LC.Append(
		fx.Hook{
			OnStart: func(ctx context.Context) error {
				defer func() {
					if r := recover(); nil != r {
						logger.Error("stacktrace from panic", zap.String("stacktrace", string(debug.Stack())), zap.Any("panic", r))
					}
				}()

				logger.Info("starting ws")
				in.WS.Start()

				return nil
			},
			OnStop: func(ctx context.Context) error {
				defer func() {
					if r := recover(); nil != r {
						logger.Error("stacktrace from panic", zap.String("stacktrace", string(debug.Stack())), zap.Any("panic", r))
					}

					if err := in.Shutdowner.Shutdown(); err != nil {
						logger.Error("encountered error trying to shutdown app: ", zap.Error(err))
					}
				}()

				logger.Info("stopping ws listeners")
				in.WS.Stop()
				for _, c := range in.CancelList {
					c()
				}

				return nil
			},
		},
	)
}
