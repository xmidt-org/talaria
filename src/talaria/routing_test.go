package main

import (
	"bytes"
	"context"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func testOutboundEnvelopeBadRequest(t *testing.T) {
	var (
		assert        = assert.New(t)
		envelope, err = newOutboundEnvelope(500*time.Millisecond, "(897(*&%(*&", "foobar.com", bytes.NewBufferString("hi"))
	)

	assert.Nil(envelope)
	assert.Error(err)
}

func testOutboundEnvelopeValid(t *testing.T) {
	var (
		assert        = assert.New(t)
		require       = require.New(t)
		envelope, err = newOutboundEnvelope(500*time.Millisecond, "POST", "foobar.com", bytes.NewBufferString("hi"))
	)

	require.NotNil(envelope)
	assert.Nil(err)

	require.NotNil(envelope.request)
	require.NotNil(envelope.cancel)

	assert.True(context.Background() != envelope.request.Context())
	envelope.cancel()
	<-envelope.request.Context().Done()
}

func TestOutboundEnvelope(t *testing.T) {
	t.Run("BadRequest", testOutboundEnvelopeBadRequest)
	t.Run("Valid", testOutboundEnvelopeValid)
}

func TestRouting(t *testing.T) {
}
