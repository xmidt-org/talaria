package main

import (
	"os"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testFactoryInit(t *testing.T) {
	var (
		assert = assert.New(t)
		logger = log.NewJSONLogger(log.NewSyncWriter(os.Stdout))
	)

	factory := NewDispatcherFactory(logger)
	assert.NotNil(factory)
}

func testCreateDispatcher(t *testing.T) {
	var (
		assert           = assert.New(t)
		require          = require.New(t)
		dispatcherName   = "connect"
		logger           = log.NewJSONLogger(log.NewSyncWriter(os.Stdout))
		outboundMeasures = NewTestOutboundMeasures()
		outbounder       = &Outbounder{EventEndpoints: map[string]interface{}{"another": []string{"http://endpoint1.com"}}}
	)

	factory := NewDispatcherFactory(logger)

	require.NotNil(factory)
	require.NotNil(outboundMeasures)
	require.NotNil(outbounder)

	dispatcher, outboundEnvelope, err := factory.CreateDispatcher(dispatcherName, outboundMeasures, outbounder, nil)

	assert.NotNil(dispatcher)
	assert.NotNil(outboundEnvelope)
	assert.NoError(err)

	assert.Equal(dispatcher.DispatcherName(), dispatcherName)
}

func testCreateDispatcherInvalidName(t *testing.T) {
	var (
		assert                = assert.New(t)
		require               = require.New(t)
		invalidDispatcherName = "bad-name"
		logger                = log.NewJSONLogger(log.NewSyncWriter(os.Stdout))
		outboundMeasures      = NewTestOutboundMeasures()
		outbounder            = &Outbounder{EventEndpoints: map[string]interface{}{"another": []string{"http://endpoint1.com"}}}
	)

	factory := NewDispatcherFactory(logger)

	require.NotNil(factory)
	require.NotNil(outboundMeasures)
	require.NotNil(outbounder)

	_, _, err := factory.CreateDispatcher(invalidDispatcherName, outboundMeasures, outbounder, nil)

	assert.Error(err)
}

func TestDispatcherFactory(t *testing.T) {
	t.Run("FactoryInit", testFactoryInit)
	t.Run("CreateDispatcher", testCreateDispatcher)
	t.Run("CreateDispatcherInvalidName", testCreateDispatcherInvalidName)
}
