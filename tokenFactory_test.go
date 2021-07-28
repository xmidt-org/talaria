package main

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/dgrijalva/jwt-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/xmidt-org/bascule"
	"github.com/xmidt-org/bascule/basculehttp"
	"github.com/xmidt-org/bascule/key"
)

func TestRawAttributesBearerTokenFactory(t *testing.T) {
	tests := []struct {
		description   string
		value         string
		jwtToken      *jwt.Token
		parseCalled   bool
		parseErr      error
		isValid       bool
		expectedToken bascule.Token
		expectedErr   error
	}{
		{
			description: "Success",
			value:       "abcd",
			jwtToken: jwt.NewWithClaims(jwt.SigningMethodHS256, &bascule.ClaimsWithLeeway{
				MapClaims: jwt.MapClaims(
					map[string]interface{}{
						jwtPrincipalKey: "test",
					},
				),
			}),
			parseCalled:   true,
			isValid:       true,
			expectedToken: bascule.NewToken("jwt", "test", rawAttributes(map[string]interface{}{jwtPrincipalKey: "test"})),
		},
		{
			description: "Empty Value",
			value:       "",
			expectedErr: errors.New("empty value"),
		},
		{
			description: "Parse Failed",
			value:       "abcd",
			jwtToken: jwt.NewWithClaims(jwt.SigningMethodHS256, &bascule.ClaimsWithLeeway{
				MapClaims: jwt.MapClaims(
					map[string]interface{}{
						jwtPrincipalKey: "test",
					},
				),
			}),
			parseCalled: true,
			parseErr:    errors.New("parse fail test"),
			expectedErr: errors.New("parse fail test"),
		},
		{
			description: "Invalid Token",
			value:       "abcd",
			jwtToken: jwt.NewWithClaims(jwt.SigningMethodHS256, &bascule.ClaimsWithLeeway{
				MapClaims: jwt.MapClaims(
					map[string]interface{}{
						jwtPrincipalKey: "test",
					},
				),
			}),
			parseCalled: true,
			expectedErr: basculehttp.ErrInvalidToken,
		},
		{
			description: "Unexpected Claims",
			value:       "abcd",
			jwtToken: jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims(
				map[string]interface{}{
					jwtPrincipalKey: "test",
				},
			)),
			parseCalled: true,
			isValid:     true,
			expectedErr: basculehttp.ErrUnexpectedClaims,
		},
		{
			description: "Principal Value not Found",
			value:       "abcd",
			jwtToken: jwt.NewWithClaims(jwt.SigningMethodHS256, &bascule.ClaimsWithLeeway{
				MapClaims: jwt.MapClaims(
					map[string]interface{}{
						"test": "test",
					},
				),
			}),
			parseCalled: true,
			isValid:     true,
			expectedErr: basculehttp.ErrInvalidPrincipal,
		},
		{
			description: "Principal Value not a String",
			value:       "abcd",
			jwtToken: jwt.NewWithClaims(jwt.SigningMethodHS256, &bascule.ClaimsWithLeeway{
				MapClaims: jwt.MapClaims(
					map[string]interface{}{
						jwtPrincipalKey: 123,
					},
				),
			}),
			parseCalled: true,
			isValid:     true,
			expectedErr: basculehttp.ErrInvalidPrincipal,
		},
	}

	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {
			assert := assert.New(t)
			r := new(key.MockResolver)
			p := new(mockJWTParser)

			if tc.parseCalled {
				p.On("ParseJWT", mock.Anything, mock.Anything, mock.Anything).Return(tc.jwtToken, tc.parseErr).Once()
				tc.jwtToken.Valid = tc.isValid
			}

			f := RawAttributesBearerTokenFactory{
				DefaultKeyID: "default-key",
				Resolver:     r,
				Parser:       p,
			}

			req := httptest.NewRequest("get", "/", nil)
			token, err := f.ParseAndValidate(context.Background(), req, "", tc.value)

			assert.Equal(tc.expectedToken, token)

			if tc.expectedErr == nil || err == nil {
				assert.Equal(tc.expectedErr, err)
			} else {
				assert.Contains(err.Error(), tc.expectedErr.Error())
			}
		})
	}
}

func TestDefaultKeyFunc(t *testing.T) {
	defaultKeyID := "default-key"

	tests := []struct {
		description       string
		token             *jwt.Token
		useDefaultKey     bool
		resolveErr        error
		expectedPublicKey interface{}
		expectedErr       error
	}{
		{
			description:       "Success",
			token:             jwt.New(jwt.SigningMethodHS256),
			expectedPublicKey: "public-key",
		},
		{
			description:       "Success with Default Key",
			useDefaultKey:     true,
			token:             jwt.New(jwt.SigningMethodHS256),
			expectedPublicKey: "public-key",
		},
		{
			description: "Resolve Error",
			token:       jwt.New(jwt.SigningMethodHS256),
			resolveErr:  errors.New("resolve error"),
			expectedErr: errors.New("resolve error"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {
			assert := assert.New(t)
			r := new(key.MockResolver)
			pair := new(key.MockPair)

			pair.On("Public").Return(tc.expectedPublicKey).Once()

			if tc.useDefaultKey {
				r.On("ResolveKey", mock.Anything, defaultKeyID).Return(pair, tc.resolveErr).Once()
			} else {
				tc.token.Header = map[string]interface{}{
					"kid": "some-value",
				}
				r.On("ResolveKey", mock.Anything, "some-value").Return(pair, tc.resolveErr).Once()
			}

			publicKey, err := defaultKeyFunc(context.Background(), defaultKeyID, r)(tc.token)

			assert.Equal(tc.expectedPublicKey, publicKey)

			if tc.expectedErr == nil || err == nil {
				assert.Equal(tc.expectedErr, err)
			} else {
				assert.Contains(err.Error(), tc.expectedErr.Error())
			}
		})
	}
}
