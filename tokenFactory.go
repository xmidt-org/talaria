package main

import (
	"context"
	"errors"
	"net/http"

	"github.com/dgrijalva/jwt-go"
	"github.com/goph/emperror"
	"github.com/xmidt-org/bascule"
	"github.com/xmidt-org/bascule/basculehttp"
	"github.com/xmidt-org/bascule/key"
)

const (
	jwtPrincipalKey = "sub"
)

// RawAttributesBearerTokenFactory parses and does basic validation for a JWT token, creating a token with RawAttributes
// instead of bascule processed attributes
type RawAttributesBearerTokenFactory struct {
	DefaultKeyID string
	Resolver     key.Resolver
	Parser       bascule.JWTParser
	Leeway       bascule.Leeway
}

func defaultKeyFunc(ctx context.Context, defaultKeyID string, keyResolver key.Resolver) jwt.Keyfunc {
	return func(token *jwt.Token) (interface{}, error) {
		keyID, ok := token.Header["kid"].(string)
		if !ok {
			keyID = defaultKeyID
		}

		pair, err := keyResolver.ResolveKey(ctx, keyID)
		if err != nil {
			return nil, emperror.Wrap(err, "failed to resolve key")
		}
		return pair.Public(), nil
	}
}

// ParseAndValidate expects the given value to be a JWT with a kid header.  The
// kid should be resolvable by the Resolver and the JWT should be Parseable and
// pass any basic validation checks done by the Parser.  If everything goes
// well, a Token of type "jwt" is returned.
func (r RawAttributesBearerTokenFactory) ParseAndValidate(ctx context.Context, _ *http.Request, _ bascule.Authorization, value string) (bascule.Token, error) {
	if len(value) == 0 {
		return nil, errors.New("empty value")
	}

	leewayclaims := bascule.ClaimsWithLeeway{
		MapClaims: make(jwt.MapClaims),
		Leeway:    r.Leeway,
	}

	jwtToken, err := r.Parser.ParseJWT(value, &leewayclaims, defaultKeyFunc(ctx, r.DefaultKeyID, r.Resolver))
	if err != nil {
		return nil, emperror.Wrap(err, "failed to parse JWT")
	}
	if !jwtToken.Valid {
		return nil, basculehttp.ErrorInvalidToken
	}

	claims, ok := jwtToken.Claims.(*bascule.ClaimsWithLeeway)

	if !ok {
		return nil, emperror.Wrap(basculehttp.ErrorUnexpectedClaims, "failed to parse JWT")
	}

	claimsMap, err := claims.GetMap()
	if err != nil {
		return nil, emperror.WrapWith(err, "failed to get map of claims", "claims struct", claims)
	}

	jwtClaims := NewRawAttributes(claimsMap)

	principalVal, ok := jwtClaims.Get(jwtPrincipalKey)
	if !ok {
		return nil, emperror.WrapWith(basculehttp.ErrorInvalidPrincipal, "principal value not found", "principal key", jwtPrincipalKey, "jwtClaims", claimsMap)
	}
	principal, ok := principalVal.(string)
	if !ok {
		return nil, emperror.WrapWith(basculehttp.ErrorInvalidPrincipal, "principal value not a string", "principal", principalVal)
	}

	return bascule.NewToken("jwt", principal, jwtClaims), nil
}
