package main

import (
	"context"
	"errors"
	"testing"

	// nolint:staticcheck

	"github.com/go-kit/kit/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/xmidt-org/webpa-common/v2/device"
	"go.uber.org/zap/zaptest"

	// nolint:staticcheck
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
				testLogger         = zaptest.NewLogger(t)
				counter            = newTestCounter()
				expectedLabels     = getLabelMaps(testCase.ExpectedError, testCase.IsFatal, strict, testCase.BaseLabelPairs)

				wrpMsg = &wrp.Message{
					PartnerIDs:  []string{"comcast", "nbc", "sky"},
					Destination: testCase.DeviceID,
				}
			)

			testMetadata := getTestDeviceMetadata()

			mockDeviceRegistry.On("Get", device.ID(testCase.DeviceID)).Return(mockDevice, !testCase.MissingDevice).Once()
			mockDevice.On("Metadata").Return(testMetadata).Once()
			mockBinOp.On("name").Return("mockBinOP")

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
				logger:        testLogger,
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
				reasonLabel: invalidWRPDest,
			},
			ExpectedError: errInvalidWRPDestination,
			IsFatal:       true,
		},
		{
			Name:          "Device not found",
			DeviceID:      "mac:112233445566",
			MissingDevice: true,
			BaseLabelPairs: map[string]string{
				reasonLabel: deviceNotFound,
			},
			ExpectedError: errDeviceNotFound,
			IsFatal:       true,
		},
		{
			Name:     "Device Credential Missing",
			DeviceID: "mac:112233445566",
			BaseLabelPairs: map[string]string{
				reasonLabel: missingDeviceCredential,
			},
			MissingDeviceCredential: true,
			ExpectedError:           errDeviceCredentialMissing,
		},

		{
			Name:     "WRP Credential Missing",
			DeviceID: "mac:112233445566",
			BaseLabelPairs: map[string]string{
				reasonLabel: missingWRPCredential,
			},
			MissingWRPCredential: true,
			ExpectedError:        errWRPCredentialsMissing,
		},

		{
			Name:     "Second Check doesn't complete",
			DeviceID: "mac:112233445566",
			BaseLabelPairs: map[string]string{
				reasonLabel: incompleteCheck,
			},
			IncompleteCheck: true,
			ExpectedError:   errIncompleteCheck,
		},
		{
			Name:     "Unauthorized Device Access",
			DeviceID: "mac:112233445566",
			BaseLabelPairs: map[string]string{
				reasonLabel: denied,
			},
			Authorized:    false,
			ExpectedError: errDeniedDeviceAccess,
		},

		{
			Name:     "Authorized Device Access",
			DeviceID: "mac:112233445566",
			BaseLabelPairs: map[string]string{
				reasonLabel: authorized,
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

func getLabelMaps(err error, isFatal, strict bool, baseLabelPairs map[string]string) map[string]string {
	out := make(map[string]string)

	for k, v := range baseLabelPairs {
		out[k] = v
	}

	outcome := accepted

	if err != nil && (isFatal || strict) {
		outcome = rejected
	}
	out[outcomeLabel] = outcome

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

func getTestDeviceMetadata() *device.Metadata {
	metadata := new(device.Metadata)
	claims := map[string]interface{}{
		device.PartnerIDClaimKey: "sky",
		device.TrustClaimKey:     100,
		"nested":                 map[string]interface{}{"happy": true},
	}

	metadata.SetClaims(claims)
	return metadata
}

func getFirstMissingDeviceCredentialChecks(t *testing.T, m *mockBinOp) []*parsedCheck {
	m.AssertNotCalled(t, "evaluate", mock.Anything, mock.Anything)

	baseChecks := getBaseChecks(m)
	baseChecks[0].deviceCredentialPath = "path>not>found"
	return baseChecks
}

func getSecondCheckMissingWRPCredentiaChecks(t *testing.T, m *mockBinOp) []*parsedCheck {
	m.On("evaluate", 100, 100).Return(true, error(nil)).Once()
	m.AssertNotCalled(t, "evaluate", []string{"comcast", "nbc", "sky"}, "sky")
	m.AssertNotCalled(t, "evaluate", true, true)

	baseChecks := getBaseChecks(m)
	baseChecks[1].wrpCredentialPath = "path>not>found"
	return baseChecks
}

func getSecondCheckWithInputValueChecks(t *testing.T, m *mockBinOp) []*parsedCheck {
	m.On("evaluate", 100, 100).Return(true, error(nil)).Once()
	m.On("evaluate", []string{"universal"}, "sky").Return(true, error(nil)).Once()
	m.On("evaluate", true, true).Return(true, error(nil)).Once()
	m.AssertNotCalled(t, "evaluate", []string{"comcast", "nbc", "sky"}, "sky")

	baseChecks := getBaseChecks(m)
	baseChecks[1].inputValue = []string{"universal"}
	return baseChecks
}

func getChecks(t *testing.T, m *mockBinOp, secondCheckIncomplete, thirdCheckAuthorized bool) []*parsedCheck {
	m.On("evaluate", 100, 100).Return(true, error(nil)).Once()
	if secondCheckIncomplete {
		m.On("evaluate", []string{"comcast", "nbc", "sky"}, "sky").Return(false, errors.New("Could not complete check")).Once()
		m.AssertNotCalled(t, "evaluate", mock.Anything, mock.Anything)
		return getBaseChecks(m)
	}

	m.On("evaluate", []string{"comcast", "nbc", "sky"}, "sky").Return(true, error(nil)).Once()
	m.On("evaluate", true, true).Return(thirdCheckAuthorized, error(nil))
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
			wrpCredentialPath:    "PartnerIDs",
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
