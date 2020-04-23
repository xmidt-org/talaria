package main

import (
	"context"
	"errors"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/xmidt-org/webpa-common/device"
	"github.com/xmidt-org/webpa-common/logging"
	"github.com/xmidt-org/wrp-go/v3"
)

type deviceAccessTestCase struct {
	Name                    string
	DeviceID                string
	MissingDevice           bool
	MissingDeviceCredential bool
	MissingWRPCredential    bool
	IncompleteCheck         bool
	InputValueCheck         bool
	Authorized              bool
	ExpectedError           error
	IsFatal                 bool
	BaseLabelPairs          map[string]string
}

func testAuthorizeWRP(t *testing.T, testCases []deviceAccessTestCase, strict bool) {
	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			var (
				assert = assert.New(t)

				mockDeviceRegistry = new(device.MockRegistry)
				mockDevice         = new(device.MockDevice)
				mockBinOp          = new(mockBinOp)
				testLogger         = logging.NewTestLogger(nil, t)
				counter            = newTestCounter()
				expectedLabels     = getLabelMaps(testCase.ExpectedError, testCase.IsFatal, strict, testCase.BaseLabelPairs)

				wrpMsg = &wrp.Message{
					PartnerIDs:  []string{"comcast", "nbc", "sky"},
					Destination: testCase.DeviceID,
				}
			)

			mockDeviceRegistry.On("Get", device.ID(testCase.DeviceID)).Return(mockDevice, !testCase.MissingDevice).Once()
			mockDevice.On("Metadata").Return(getTestDeviceMetadata()).Once()
			mockBinOp.On("Name").Return("mockBinOP")

			var checks []*parsedCheck
			if testCase.MissingDeviceCredential {
				checks = getFirstMissingDeviceCredentialChecks(t, mockBinOp)
			} else if testCase.MissingWRPCredential {
				checks = getSecondCheckMissingWRPCredentiaChecks(t, mockBinOp)
			} else if testCase.InputValueCheck {
				checks = getSecondCheckWithInputValueChecks(t, mockBinOp)
			} else {
				checks = getChecks(t, mockBinOp, testCase.IncompleteCheck, testCase.Authorized)
			}

			deviceAccessAuthority := &talariaDeviceAccess{
				strict:             strict,
				wrpMessagesCounter: counter,
				deviceRegistry:     mockDeviceRegistry,
				sep:                ">",
				debugLogger:        logging.Debug(testLogger),
				checks:             checks,
			}

			err := deviceAccessAuthority.authorizeWRP(context.Background(), wrpMsg)
			if strict || testCase.IsFatal {
				assert.Equal(testCase.ExpectedError, err)
			} else {
				assert.Nil(err)
			}
			assert.Equal(float64(1), counter.count)
			assert.Equal(expectedLabels, counter.labelPairs)
		})
	}
}

func TestAuthorizeWRP(t *testing.T) {
	testCases := []deviceAccessTestCase{
		{
			Name:     "Invalid WRP Destination",
			DeviceID: "McD's:1122334455",
			BaseLabelPairs: map[string]string{
				ReasonLabel: InvalidWRPDest,
			},
			ExpectedError: ErrInvalidWRPDestination,
			IsFatal:       true,
		},
		{
			Name:          "Device not found",
			DeviceID:      "mac:112233445566",
			MissingDevice: true,
			BaseLabelPairs: map[string]string{
				ReasonLabel: DeviceNotFound,
			},
			ExpectedError: ErrDeviceNotFound,
			IsFatal:       true,
		},
		{
			Name:     "Device Credential Missing",
			DeviceID: "mac:112233445566",
			BaseLabelPairs: map[string]string{
				ReasonLabel: MissingDeviceCredential,
			},
			MissingDeviceCredential: true,
			ExpectedError:           ErrDeviceCredentialMissing,
		},

		{
			Name:     "WRP Credential Missing",
			DeviceID: "mac:112233445566",
			BaseLabelPairs: map[string]string{
				ReasonLabel: MissingWRPCredential,
			},
			MissingWRPCredential: true,
			ExpectedError:        ErrWRPCredentialsMissing,
		},

		{
			Name:     "Second Check doesn't complete",
			DeviceID: "mac:112233445566",
			BaseLabelPairs: map[string]string{
				ReasonLabel: IncompleteCheck,
			},
			IncompleteCheck: true,
			ExpectedError:   ErrIncompleteCheck,
		},
		{
			Name:     "Unauthorized Device Access",
			DeviceID: "mac:112233445566",
			BaseLabelPairs: map[string]string{
				ReasonLabel: Denied,
			},
			Authorized:    false,
			ExpectedError: ErrDeniedDeviceAccess,
		},

		{
			Name:     "Authorized Device Access",
			DeviceID: "mac:112233445566",
			BaseLabelPairs: map[string]string{
				ReasonLabel: Authorized,
			},
			Authorized: true,
		},
	}
	t.Run("strict", func(t *testing.T) {
		testAuthorizeWRP(t, testCases, true)
	})
	t.Run("lenient", func(t *testing.T) {
		testAuthorizeWRP(t, testCases, false)
	})
}

func getDeviceAccessAuthority(logger log.Logger, counter metrics.Counter, deviceRegistry device.Registry, strict bool) deviceAccess {
	return &talariaDeviceAccess{
		strict:             strict,
		wrpMessagesCounter: counter,
		deviceRegistry:     deviceRegistry,
		sep:                ">",
		debugLogger:        logging.Debug(logger),
	}
}

func getLabelMaps(err error, isFatal, strict bool, baseLabelPairs map[string]string) map[string]string {
	out := make(map[string]string)

	for k, v := range baseLabelPairs {
		out[k] = v
	}

	outcome := Accepted

	if err != nil && (isFatal || strict) {
		outcome = Rejected
	}
	out[OutcomeLabel] = outcome

	return out
}

type testCounter struct {
	count      float64
	labelPairs map[string]string
}

func (c *testCounter) Add(delta float64) {
	c.count += delta
}

func (c *testCounter) With(labelValues ...string) metrics.Counter {
	for i := 0; i < len(labelValues)-1; i += 2 {
		c.labelPairs[labelValues[i]] = labelValues[i+1]
	}
	return c
}

func newTestCounter() *testCounter {
	return &testCounter{
		labelPairs: make(map[string]string),
	}
}

func getTestDeviceMetadata() device.Metadata {
	var metadata = device.NewDeviceMetadata()
	claims := map[string]interface{}{
		device.PartnerIDClaimKey: "sky",
		device.TrustClaimKey:     100,
		"nested":                 map[string]interface{}{"happy": true},
	}

	metadata.SetJWTClaims(device.NewJWTClaims(claims))
	return metadata
}

func getFirstMissingDeviceCredentialChecks(t *testing.T, m *mockBinOp) []*parsedCheck {
	m.AssertNotCalled(t, "Evaluate", mock.Anything, mock.Anything)

	baseChecks := getBaseChecks(m)
	baseChecks[0].deviceCredentialPath = "path>not>found"
	return baseChecks
}

func getSecondCheckMissingWRPCredentiaChecks(t *testing.T, m *mockBinOp) []*parsedCheck {
	m.On("Evaluate", 100, 100).Return(true, error(nil)).Once()
	m.AssertNotCalled(t, "Evaluate", []string{"comcast", "nbc", "sky"}, "sky")
	m.AssertNotCalled(t, "Evaluate", true, true)

	baseChecks := getBaseChecks(m)
	baseChecks[1].wrpCredentialPath = "path>not>found"
	return baseChecks
}

func getSecondCheckWithInputValueChecks(t *testing.T, m *mockBinOp) []*parsedCheck {
	m.On("Evaluate", 100, 100).Return(true, error(nil)).Once()
	m.On("Evaluate", []string{"universal"}, "sky").Return(true, error(nil)).Once()
	m.On("Evaluate", true, true).Return(true, error(nil)).Once()

	baseChecks := getBaseChecks(m)
	baseChecks[1].inputValue = []string{"universal"}
	return baseChecks
}

func getChecks(t *testing.T, m *mockBinOp, secondCheckIncomplete, thirdCheckAuthorized bool) []*parsedCheck {
	m.On("Evaluate", 100, 100).Return(true, error(nil)).Once()
	if secondCheckIncomplete {
		m.On("Evaluate", []string{"comcast", "nbc", "sky"}, "sky").Return(false, errors.New("Could not complete check")).Once()
		m.AssertNotCalled(t, "Evaluate", mock.Anything, mock.Anything)
		return getBaseChecks(m)
	}

	m.On("Evaluate", []string{"comcast", "nbc", "sky"}, "sky").Return(true, error(nil)).Once()
	m.On("Evaluate", true, true).Return(thirdCheckAuthorized, error(nil))
	return getBaseChecks(m)
}

func getBaseChecks(m *mockBinOp) []*parsedCheck {
	return []*parsedCheck{
		{
			name:                 "trustedDevice",
			deviceCredentialPath: device.TrustClaimKey,
			assertion:            m,
			inputValue:           100,
		},
		{
			name:                 "partnerID",
			deviceCredentialPath: device.PartnerIDClaimKey,
			wrpCredentialPath:    "partnerIDs",
			assertion:            m,
			inversed:             true,
		},
		{
			name:                 "happyDevice",
			deviceCredentialPath: "nested>happy",
			assertion:            m,
			inputValue:           true,
		},
	}
}
