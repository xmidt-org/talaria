package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/goph/emperror"
	"github.com/xmidt-org/bascule"
	"github.com/xmidt-org/bascule/basculehttp"
	"github.com/xmidt-org/webpa-common/device"
)

type AttributedBasicTokenFactory struct {
	VerifiedJWTTokenHeaderName string
	TargetAttributeKey         string
	UserPassMap                map[string]string
}

//token factory which produces basic auth token with an unverified jwt token in its attribute
func (a *AttributedBasicTokenFactory) ParseAndValidate(ctx context.Context, r *http.Request, auth bascule.Authorization, value string) (bascule.Token, error) {
	basicTokenFactory := basculehttp.BasicTokenFactory(a.UserPassMap)
	token, err := basicTokenFactory.ParseAndValidate(ctx, r, auth, value)
	if err != nil {
		return nil, err
	}

	attributes := map[string]interface{}(token.Attributes())
	jwtToken, err := ParseNotVerifyJWT(r.Header.Get(a.VerifiedJWTTokenHeaderName))
	if err != nil {
		return nil, err
	}

	attributes[a.TargetAttributeKey] = jwtToken

	return bascule.NewToken(token.Type(), token.Principal(), attributes), nil
}

//UnverifiedBearerTokenFactory simply parses the jwt token without
//validating either the claims or the signature.
//This factory is intended to be used in cases where the token source
//is trusted to have already performed such checks
//TODO: another approach would be to add flags to the existing BearerTokenFactory such as
//SkipSignatureVerification
func ParseNotVerifyJWT(value string) (bascule.Token, error) {
	if len(value) == 0 {
		return nil, errors.New("empty value")
	}

	parser := jwt.Parser{
		SkipClaimsValidation: true,
	}

	leewayclaims := bascule.ClaimsWithLeeway{
		MapClaims: make(jwt.MapClaims),
	}

	jwsToken, _, err := parser.ParseUnverified(value, &leewayclaims)

	if err != nil {
		return nil, emperror.Wrap(err, "failed to parse JWS")
	}

	claims, ok := jwsToken.Claims.(*bascule.ClaimsWithLeeway)
	if !ok {
		return nil, emperror.Wrap(basculehttp.ErrorUnexpectedClaims, "failed to parse JWS")
	}

	claimsMap, err := claims.GetMap()
	if err != nil {
		return nil, emperror.WrapWith(err, "failed to get map of claims", "claims struct", claims)
	}

	payload := bascule.Attributes(claimsMap)

	principal, ok := payload["sub"].(string)
	if !ok {
		return nil, emperror.WrapWith(basculehttp.ErrorUnexpectedPrincipal, "failed to get and convert principal", "principal", payload["sub"], "payload", payload)
	}

	return bascule.NewToken("jwt", principal, payload), nil

}

type ApiAccessToDeviceCheck struct {
	APITablePath    string
	Op              string
	DeviceTablePath string
}

type ApiAccessToDeviceValidator struct {
	manager device.Manager
	Checks  []ApiAccessToDeviceCheck
}

func createDeviceMetadata(manager device.Manager, deviceID device.ID) (map[string]interface{}, error) {
	m := make(map[string]interface{})
	d, ok := manager.Get(deviceID)
	if !ok {
		return nil, errors.New("Device not longer connected")
	}
	fmt.Printf("device metadata %v\n", d)
	m["partner-ids"] = d.PartnerIDs()
	return m, nil
}

func (a *ApiAccessToDeviceValidator) Check(ctx context.Context, token bascule.Token) error {
	var APIPermissionsMap map[string]interface{} = token.Attributes()

	deviceID, ok := device.GetID(ctx)
	fmt.Printf("Running checks for device %v, Checks %v\n", deviceID, a.Checks)

	if !ok {
		return errors.New("Device ID could not be fetched from incoming request")
	}
	DeviceMetadataMap, err := createDeviceMetadata(a.manager, deviceID)

	if err != nil {
		//TODO: give more context
		return err
	}

	for _, check := range a.Checks {
		switch check.Op {
		case "intersect":
			fmt.Printf("Api table: %v\n device table: %v\n", APIPermissionsMap, DeviceMetadataMap)
			ok, err := Intersect(APIPermissionsMap, DeviceMetadataMap, check.APITablePath, check.DeviceTablePath)
			if !ok {
				return errors.New("error blah blah failed")
			}

			if err != nil {
				//TODO: give more context
				return err
			}
		}
	}
	return nil

}
