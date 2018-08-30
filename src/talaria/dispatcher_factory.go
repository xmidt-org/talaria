package main

import (
	"errors"
	"fmt"
	"github.com/Comcast/webpa-common/logging"
	"github.com/go-kit/kit/log"
	"strings"
)

// CreateDispatcherFactory is a type alias for creating new dispatcher types.
type CreateDispatcherFactory func(om OutboundMeasures, o *Outbounder, urlFilter URLFilter) (Dispatcher, <-chan outboundEnvelope, error)

type dispatcherRegistry struct {
	errorLog    log.Logger
	infoLog     log.Logger
	dispatchers map[string]CreateDispatcherFactory
}

func initRegistry(logger log.Logger) *dispatcherRegistry {
	dispatcherFactories := make(map[string]CreateDispatcherFactory)

	dRegistry := &dispatcherRegistry {
		errorLog:    logging.Error(logger),
		infoLog:     logging.Info(logger),
		dispatchers: dispatcherFactories,
	}

	dRegistry.register(MessageReceivedDispatcher, NewDispatcher)
	dRegistry.register(ConnectDispatcher, NewConnectDispatcher)
	dRegistry.register(DisconnectDispatcher, NewDisconnectDispatcher)

	return dRegistry
}

func (r *dispatcherRegistry) register(name string, factory CreateDispatcherFactory) {
	if factory == nil {
		r.errorLog.Log(logging.MessageKey(), "Error registering dispatcher factory", "name", name)
	}

	_, registered := r.dispatchers[name]

	if registered {
		r.errorLog.Log(logging.MessageKey(), "Already registered dispatcher ", "name", name)
	}

	r.infoLog.Log(logging.MessageKey(), "Registered dispatcher", "name", name)
	r.dispatchers[name] = factory
}

type Factory interface {
	CreateDispatcher(name string, om OutboundMeasures, o *Outbounder, urlFilter URLFilter) (Dispatcher, <-chan outboundEnvelope, error)
}

type dispatcherFactory struct {
	dispatcherRegistry *dispatcherRegistry
}

func NewDispatcherFactory(logger log.Logger) Factory {
	return &dispatcherFactory{initRegistry(logger)}
}

func (f *dispatcherFactory) CreateDispatcher(name string, om OutboundMeasures, o *Outbounder, urlFilter URLFilter) (Dispatcher, <-chan outboundEnvelope, error) {
	dispatcher, ok := f.dispatcherRegistry.dispatchers[name]

	if !ok {
		availableDispatchers := make([]string, 0, len(f.dispatcherRegistry.dispatchers))

		for d := range f.dispatcherRegistry.dispatchers {
			availableDispatchers = append(availableDispatchers, d)
		}

		return nil, nil, errors.New(fmt.Sprintf(
			"Invalid dispatcher name. Available dispatchers: %s",
			strings.Join(availableDispatchers, ", ")))
	}

	return dispatcher(om, o, urlFilter)
}
