/**
 * Copyright 2017 Comcast Cable Communications Management, LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */
package main

import (
	"context"

	"github.com/dgrijalva/jwt-go"
	"github.com/stretchr/testify/mock"
	"github.com/xmidt-org/webpa-common/device"
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
	arguments := parser.Called(token)
	jwtToken, _ := arguments.Get(0).(*jwt.Token)
	return jwtToken, arguments.Error(1)
}
