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

/*
 Component to help build the token we do validation on
*/

//AttributedBasicTokenFactory enriches bascule's BasicTokenFactory
//by providing a way to fetch a JWT token from the incoming request
//Such JWT token is assumed to have a valid signature already
//(still subject to change)
type AttributedBasicTokenFactory struct {
	VerifiedJWTTokenHeaderName string
	TargetAttributeKey         string
	UserPassMap                map[string]string
}

//ParseAndValidate ensures a basic auth token can be built with a JWT token
//passed in its attributes
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

//ParseNotVerifyJWT parses a JWT into a bascule Token skipping claims and signature verification
func ParseNotVerifyJWT(value string) (bascule.Token, error) {
	if len(value) == 0 {
		return nil, errors.New("empty value")
	}
	parts := strings.Split(value, " ")

	//sanity check
	if len(parts) != 2 {
		return nil, fmt.Errorf("incoming header was expected to be a JWT token with two parts. Header: %v", value)
	}
	tokenType, token := strings.ToLower(strings.TrimSpace(parts[0])), strings.TrimSpace(parts[1])

	if tokenType != "bearer" {
		return nil, fmt.Errorf("incoming header should be of the format: Bearer [token]. Header: %v", value)
	}

	parser := jwt.Parser{
		SkipClaimsValidation: true,
	}

	leewayclaims := bascule.ClaimsWithLeeway{
		MapClaims: make(jwt.MapClaims),
	}

	jwsToken, _, err := parser.ParseUnverified(token, &leewayclaims)

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

/*
 Helper components to run validation checks
*/

//MapLoader allows a client that's aware of the structure of a map
//to fetch values from such map given a list of keys to follow
type MapLoader interface {
	Load(map[string]interface{}, Path) interface{}
}

type simpleMapLoader struct{}

func (s *simpleMapLoader) Load(m map[string]interface{}, keyPath Path) interface{} {
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

//DeviceAccessCheck describes a single unit of  assertion check between
//values presented by API users and the device
type DeviceAccessCheck struct {
	//UserCredentialPath is the Sep-delimited path to the credential value
	//presented by API users attempting to contact a device.
	UserCredentialPath string

	//Op is the string describing the operation that should be run for this
	//check (i.e. contains)
	Op string

	//DeviceCredentialPath is the Sep-delimited path to the credential value
	//associated with the device
	DeviceCredentialPath string

	//Sep is the separator to be used to split the keys from the given paths
	//(Optional. Defaults to '.')
	Sep string

	//Inversed should be set to true if Op should be applied from
	//valueAt(DeviceCredentialPath) to valueAt(UserCredentialPath)
	//(Optional)
	Inversed bool
}

//DeviceAccessValidator provides a strategy to limit device access to users
//based on credentials the device presents at registration and the user presents
//when making a request.
//TODO: Potential future feature:
//- Today, we check user credentials strictly against device credentials. If
//there is a use case for checking against values directly provided by talaria during
//config, it should not be a huge
type DeviceAccessValidator struct {
	//APIJWTHeaderName is the HTTP header name from which the user credentials
	APIJWTHeaderName string

	//Checks is the list of checks this validator should run
	Checks []DeviceAccessCheck

	//Values configured at runtime
	manager              device.Manager
	jwtTokenAttributeKey string
	checkParser          *checkParser
	parsedChecks         []parsedCheck
	mapLoader            MapLoader
}

func (a *DeviceAccessValidator) parseChecks() error {
	parsedChecks, err := a.checkParser.parse(a.Checks)
	if err != nil {
		return err
	}
	a.parsedChecks = parsedChecks
	return nil
}

//Check runs all configured assertions against incoming HTTP requests which
//attempt to interact with a device.
func (a *DeviceAccessValidator) Check(ctx context.Context, token bascule.Token) error {
	if len(a.parsedChecks) < 1 {
		return nil
	}

	jwtTokenVal, ok := token.Attributes().Get(a.jwtTokenAttributeKey)

	if !ok {
		return errors.New("could not find JWT in token attritube")
	}

	jwtToken, ok := jwtTokenVal.(bascule.Token)

	if !ok {
		return errors.New("expected value for token to be bascule.Token")
	}

	userCredentials := jwtToken.Attributes()

	deviceID, ok := device.GetID(ctx)

	if !ok {
		return errors.New("Device ID could not be fetched from incoming request")
	}

	deviceCredentials, err := fetchDeviceJWTClaims(a.manager, deviceID)

	if err != nil {
		//TODO: give more context
		return err
	}

	for _, parsedCheck := range a.parsedChecks {
		this := a.mapLoader.Load(userCredentials, parsedCheck.userCredentialPath)
		that := a.mapLoader.Load(deviceCredentials, parsedCheck.deviceCredentialPath)

		if parsedCheck.inversed {
			this, that = that, this
		}

		ok, err := parsedCheck.assertion.Run(this, that)
		if err != nil {
			return err
		}

		if !ok {
			return errors.New("device access check failed")
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
