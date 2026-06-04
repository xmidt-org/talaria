// SPDX-FileCopyrightText: 2017 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"errors"
	"testing"

	"github.com/golang-jwt/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/xmidt-org/bascule"
)

func mustSignToken(t *testing.T, claims jwt.MapClaims, kid string, secret []byte) string {
	t.Helper()

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	if kid != "" {
		token.Header["kid"] = kid
	}

	raw, err := token.SignedString(secret)
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	return raw
}

func TestRawAttributesBearerTokenParser(t *testing.T) {
	tests := []struct {
		description       string
		value             string
		defaultKeyID      string
		resolvedKeyID     string
		resolverKey       []byte
		resolverErr       error
		expectedErr       error
		expectedPrincipal string
		expectedSub       string
	}{
		{
			description:       "Success",
			defaultKeyID:      "default-key",
			resolvedKeyID:     "kid-1",
			resolverKey:       []byte("secret"),
			expectedPrincipal: "principal-1",
			expectedSub:       "principal-1",
		},
		{
			description: "Empty Value",
			value:       "",
			expectedErr: bascule.ErrMissingCredentials,
		},
		{
			description: "Malformed Token",
			value:       "not-a-jwt",
			expectedErr: bascule.ErrInvalidCredentials,
		},
		{
			description:   "Invalid Signature",
			defaultKeyID:  "default-key",
			resolvedKeyID: "kid-2",
			resolverKey:   []byte("wrong-secret"),
			expectedErr:   bascule.ErrInvalidCredentials,
		},
		{
			description:   "Missing Principal",
			defaultKeyID:  "default-key",
			resolvedKeyID: "kid-3",
			resolverKey:   []byte("secret"),
			expectedErr:   bascule.ErrInvalidCredentials,
		},
		{
			description:   "Expired Token",
			defaultKeyID:  "default-key",
			resolvedKeyID: "kid-4",
			resolverKey:   []byte("secret"),
			expectedErr:   bascule.ErrBadCredentials,
		},
	}

	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {
			assert := assert.New(t)

			resolver := new(MockResolver)
			if tc.resolvedKeyID != "" {
				pair := new(mockKey)
				pair.On("Public").Return(tc.resolverKey).Once()
				resolver.On("Resolve", mock.Anything, tc.resolvedKeyID).Return(pair, tc.resolverErr).Once()
			}

			value := tc.value
			switch tc.description {
			case "Success":
				value = mustSignToken(t, jwt.MapClaims{jwtPrincipalKey: tc.expectedSub}, tc.resolvedKeyID, tc.resolverKey)
			case "Invalid Signature":
				value = mustSignToken(t, jwt.MapClaims{jwtPrincipalKey: "principal-2"}, tc.resolvedKeyID, []byte("different-secret"))
			case "Missing Principal":
				value = mustSignToken(t, jwt.MapClaims{"foo": "bar"}, tc.resolvedKeyID, tc.resolverKey)
			case "Expired Token":
				value = mustSignToken(t, jwt.MapClaims{jwtPrincipalKey: "principal-4", "exp": float64(1)}, tc.resolvedKeyID, tc.resolverKey)
			}

			parser := RawAttributesBearerTokenParser{
				DefaultKeyID: tc.defaultKeyID,
				Resolver:     resolver,
			}

			token, err := parser.Parse(context.Background(), value)
			if tc.expectedErr == nil {
				if !assert.NoError(err) {
					return
				}
				if assert.NotNil(token) {
					assert.Equal(tc.expectedPrincipal, token.Principal())
					accessor, ok := token.(bascule.AttributesAccessor)
					if assert.True(ok) {
						sub, found := accessor.Get(jwtPrincipalKey)
						assert.True(found)
						assert.Equal(tc.expectedSub, sub)
					}
				}
			} else {
				assert.Nil(token)
				assert.ErrorIs(err, tc.expectedErr)
			}

			resolver.AssertExpectations(t)
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
			resolver := new(MockResolver)
			pair := new(mockKey)

			pair.On("Public").Return(tc.expectedPublicKey).Once()

			if tc.useDefaultKey {
				resolver.On("Resolve", mock.Anything, defaultKeyID).Return(pair, tc.resolveErr).Once()
			} else {
				tc.token.Header = map[string]interface{}{
					"kid": "some-value",
				}
				resolver.On("Resolve", mock.Anything, "some-value").Return(pair, tc.resolveErr).Once()
			}

			publicKey, err := defaultKeyFunc(context.Background(), defaultKeyID, resolver)(tc.token)

			assert.Equal(tc.expectedPublicKey, publicKey)
			if tc.expectedErr == nil {
				assert.NoError(err)
			} else {
				assert.Error(err)
				assert.Contains(err.Error(), tc.expectedErr.Error())
			}
		})
	}
}
