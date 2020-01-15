package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

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
	parts := strings.Split(value, " ")

	if len(parts) != 2 { //TODO: add actual raw token in error msg
		return nil, fmt.Errorf("JWT token expected to have two parts: %v", parts)
	}

	parser := jwt.Parser{
		SkipClaimsValidation: true,
	}

	leewayclaims := bascule.ClaimsWithLeeway{
		MapClaims: make(jwt.MapClaims),
	}

	jwsToken, _, err := parser.ParseUnverified(parts[1], &leewayclaims)

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

type mapReader interface {
	Read(map[string]interface{}, Path) interface{}
}

type simpleMapReader struct{}

func (s *simpleMapReader) Read(m map[string]interface{}, keyPath Path) interface{} {
	var v interface{}
	for _, k := range keyPath {
		var ok bool
		if v, ok = m[k]; !ok {
			return nil //we could no longer follow key from path in map
		}

		if tm, ok := v.(map[string]interface{}); ok {
			m = tm
			continue
		}
		m = nil //we reached non-map value
	}
	return v
}

type ApiAccessToDeviceCheck struct {
	APITablePath    string
	Op              string
	DeviceTablePath string
	Sep             string
	Inversed        bool
}

type ApiAccessToDeviceValidator struct {
	//Values that can be read from config
	APIJWTHeaderName string
	Checks           []ApiAccessToDeviceCheck

	//Values configured at runtime
	manager              device.Manager
	jwtTokenAttributeKey string
	checkParser          *checkParser
	parsedChecks         []parsedCheck
	mapReader            mapReader
}

func (a *ApiAccessToDeviceValidator) parseChecks() error {
	parsedChecks, err := a.checkParser.parse(a.Checks)
	if err != nil {
		return err
	}
	a.parsedChecks = parsedChecks
	return nil
}

func (a *ApiAccessToDeviceValidator) Check(ctx context.Context, token bascule.Token) error {
	if a.parsedChecks == nil || len(a.parsedChecks) < 1 {
		return nil
	}
	//TODO: attributes are being populated. Ensure you are reading correctly from it
	v, ok := token.Attributes().Get(a.jwtTokenAttributeKey)

	if !ok {
		return errors.New("Expected attritube key for token to exist")
	}

	jwtToken, ok := v.(bascule.Token)

	if !ok {
		return errors.New("Expected value for token to be bascule.Token")
	}

	APIPermissionsMap := jwtToken.Attributes()

	deviceID, ok := device.GetID(ctx)
	fmt.Printf("Running checks for device %v, Checks %v\n", deviceID, a.Checks)

	if !ok {
		return errors.New("Device ID could not be fetched from incoming request")
	}

	DeviceMetadataMap, err := fetchDeviceJWTClaims(a.manager, deviceID)

	if err != nil {
		//TODO: give more context
		return err
	}

	for _, parsedCheck := range a.parsedChecks {
		this := a.mapReader.Read(APIPermissionsMap, parsedCheck.apiTablePath)
		that := a.mapReader.Read(DeviceMetadataMap, parsedCheck.deviceTablePath)

		if parsedCheck.inversed {
			this, that = that, this
		}

		fmt.Printf("this: %v\n", this)
		fmt.Printf("that: %v\n", that)

		ok, err := parsedCheck.check.Execute(this, that)
		if err != nil {
			return err
		}

		if !ok {
			return errors.New("Check failed")
		}
	}
	return nil
}

func fetchDeviceJWTClaims(manager device.Manager, deviceID device.ID) (map[string]interface{}, error) {
	d, ok := manager.Get(deviceID)
	if !ok {
		return nil, errors.New("Device not connected")
	}

	return d.Metadata().JWTClaims().Data(), nil
}
