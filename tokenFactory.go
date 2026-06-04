// SPDX-FileCopyrightText: 2017 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"fmt"

	"github.com/golang-jwt/jwt"
	"github.com/goph/emperror"
	"github.com/xmidt-org/bascule"
	"github.com/xmidt-org/bascule/basculehttp"
	"github.com/xmidt-org/clortho"
)

const (
	jwtPrincipalKey = "sub"
	jwtTokenType    = "jwt"
)

// Leeway configures optional JWT time checks in seconds.
type Leeway struct {
	EXP int64 `json:"exp"`
	NBF int64 `json:"nbf"`
	IAT int64 `json:"iat"`
}

type tokenType interface {
	TokenType() string
}

type rawAttributesJWTToken struct {
	principal string
	claims    RawAttributes
}

func (t *rawAttributesJWTToken) Principal() string {
	return t.principal
}

func (t *rawAttributesJWTToken) Get(key string) (any, bool) {
	if t == nil || t.claims == nil {
		return nil, false
	}

	return t.claims.Get(key)
}

func (t *rawAttributesJWTToken) TokenType() string {
	return jwtTokenType
}

// RawAttributesBearerTokenParser parses and validates JWT tokens into bascule tokens
// that preserve all claims as RawAttributes.
type RawAttributesBearerTokenParser struct {
	DefaultKeyID string
	Resolver     clortho.Resolver
	Leeway       Leeway
}

func defaultKeyFunc(ctx context.Context, defaultKeyID string, resolver clortho.Resolver) jwt.Keyfunc {
	return func(token *jwt.Token) (interface{}, error) {
		keyID, ok := token.Header["kid"].(string)
		if !ok {
			keyID = defaultKeyID
		}

		pair, err := resolver.Resolve(ctx, keyID)
		if err != nil {
			return nil, emperror.Wrap(err, "failed to resolve key")
		}
		return pair.Public(), nil
	}
}

// Parse expects the given value to be a JWT with an optional kid header. The
// kid should be resolvable by the Resolver and the JWT should pass signature
// and temporal checks.
func (r RawAttributesBearerTokenParser) Parse(ctx context.Context, value string) (bascule.Token, error) {
	if len(value) == 0 {
		return nil, bascule.ErrMissingCredentials
	}

	claims := jwt.MapClaims{}
	// Skip built-in claim validation so expiration checks are handled by
	// validateTimeClaimsWithLeeway and honor configured leeway values.
	parser := jwt.Parser{SkipClaimsValidation: true}
	jwtToken, err := parser.ParseWithClaims(value, claims, defaultKeyFunc(ctx, r.DefaultKeyID, r.Resolver))
	if err != nil {
		return nil, bascule.ErrInvalidCredentials
	}
	if !jwtToken.Valid {
		return nil, bascule.ErrBadCredentials
	}

	if err := validateTimeClaimsWithLeeway(claims, r.Leeway); err != nil {
		return nil, bascule.ErrBadCredentials
	}

	claimsMap := make(map[string]interface{}, len(claims))
	for k, v := range claims {
		claimsMap[k] = v
	}

	jwtClaims := NewRawAttributes(claimsMap)

	principalVal, ok := jwtClaims.Get(jwtPrincipalKey)
	if !ok {
		return nil, bascule.ErrInvalidCredentials
	}
	principal, ok := principalVal.(string)
	if !ok || principal == "" {
		return nil, bascule.ErrInvalidCredentials
	}

	return &rawAttributesJWTToken{principal: principal, claims: jwtClaims}, nil
}

func validateTimeClaimsWithLeeway(claims jwt.MapClaims, leeway Leeway) error {
	now := jwt.TimeFunc().Unix()

	if !claims.VerifyExpiresAt(now+leeway.EXP, false) {
		return fmt.Errorf("token is expired")
	}

	if !claims.VerifyIssuedAt(now-leeway.IAT, false) {
		return fmt.Errorf("token used before issued")
	}

	if !claims.VerifyNotBefore(now-leeway.NBF, false) {
		return fmt.Errorf("token is not valid yet")
	}

	return nil
}

func nonEmptyPrincipalValidator(token bascule.Token) error {
	if token.Principal() == "" {
		return fmt.Errorf("empty token principal")
	}

	return nil
}

func validJWTTokenTypeValidator(token bascule.Token) error {
	tt, ok := token.(tokenType)
	if !ok || tt.TokenType() != jwtTokenType {
		return fmt.Errorf("unexpected token type")
	}

	return nil
}

type basicAllowedTokenParser struct {
	allowed map[string]string
}

func (batp basicAllowedTokenParser) Parse(ctx context.Context, raw string) (bascule.Token, error) {
	token, err := (basculehttp.BasicTokenParser{}).Parse(ctx, raw)
	if err != nil {
		return nil, err
	}

	var basicToken basculehttp.BasicToken
	if !bascule.TokenAs(token, &basicToken) {
		return nil, bascule.ErrInvalidCredentials
	}

	expectedPassword, ok := batp.allowed[basicToken.UserName()]
	if !ok || basicToken.Password() != expectedPassword {
		return nil, bascule.ErrBadCredentials
	}

	return token, nil
}
