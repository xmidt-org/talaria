// SPDX-FileCopyrightText: 2017 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"crypto"
	"unicode/utf8"

	"github.com/go-kit/kit/metrics"
	"github.com/golang-jwt/jwt"
	"github.com/stretchr/testify/mock"
	"github.com/xmidt-org/clortho"
	"github.com/xmidt-org/webpa-common/v2/device"
	"github.com/xmidt-org/wrp-go/v3"
)

type mockURLFilter struct {
	mock.Mock
}

func (m *mockURLFilter) Filter(v string) (string, error) {
	arguments := m.Called(v)
	return arguments.String(0), arguments.Error(1)
}

type mockRouter struct {
	mock.Mock
}

func (m *mockRouter) Route(request *device.Request) (*device.Response, error) {
	arguments := m.Called(request)
	first, _ := arguments.Get(0).(*device.Response)
	return first, arguments.Error(1)
}

type mockBinOp struct {
	mock.Mock
}

func (m *mockBinOp) evaluate(left, right interface{}) (bool, error) {
	arguments := m.Called(left, right)
	return arguments.Bool(0), arguments.Error(1)
}

func (m *mockBinOp) name() string {
	arguments := m.Called()
	return arguments.String(0)
}

type mockDeviceAccess struct {
	mock.Mock
}

func (m *mockDeviceAccess) authorizeWRP(ctx context.Context, message *wrp.Message) error {
	arguments := m.Called(ctx, message)
	return arguments.Error(0)
}

type mockJWTParser struct {
	mock.Mock
}

func (parser *mockJWTParser) ParseJWT(token string, claims jwt.Claims, parseFunc jwt.Keyfunc) (*jwt.Token, error) {
	arguments := parser.Called(token, claims, parseFunc)
	jwtToken, _ := arguments.Get(0).(*jwt.Token)
	return jwtToken, arguments.Error(1)
}

// mockHistogram provides the mock implementation of the metrics.Histogram object
type mockHistogram struct {
	mock.Mock
}

func (m *mockHistogram) Observe(value float64) {
	m.Called(value)
}

func (m *mockHistogram) With(labelValues ...string) metrics.Histogram {
	for _, v := range labelValues {
		if !utf8.ValidString(v) {
			panic("not UTF-8")
		}
	}
	m.Called(labelValues)
	return m
}

// mockCounter provides the mock implementation of the metrics.Counter object
type mockCounter struct {
	mock.Mock
}

func (m *mockCounter) Add(delta float64) {
	m.Called(delta)
}

func (m *mockCounter) With(labelValues ...string) metrics.Counter {
	for _, v := range labelValues {
		if !utf8.ValidString(v) {
			panic("not UTF-8")
		}
	}
	m.Called(labelValues)
	return m
}

// mockKey is a mock for key.
type mockKey struct {
	mock.Mock
	clortho.Thumbprinter
}

func (key *mockKey) Public() crypto.PublicKey {
	arguments := key.Called()
	return arguments.Get(0)
}

func (key *mockKey) KeyType() string {
	arguments := key.Called()
	return arguments.String(0)
}

func (key *mockKey) KeyID() string {
	arguments := key.Called()
	return arguments.String(0)
}

func (key *mockKey) KeyUsage() string {
	arguments := key.Called()
	return arguments.String(0)
}

func (key *mockKey) Raw() interface{} {
	arguments := key.Called()
	return arguments.Get(0)
}

// MockResolver is a stretchr mock for Resolver.  It's exposed for other package tests.
type MockResolver struct {
	mock.Mock
}

func (resolver *MockResolver) Resolve(ctx context.Context, keyId string) (clortho.Key, error) {
	arguments := resolver.Called(ctx, keyId)
	if key, ok := arguments.Get(0).(clortho.Key); ok {
		return key, arguments.Error(1)
	} else {
		return nil, arguments.Error(1)
	}
}
func (resolver *MockResolver) AddListener(l clortho.ResolveListener) clortho.CancelListenerFunc {
	arguments := resolver.Called(l)
	return arguments.Get(0).(clortho.CancelListenerFunc)
}
