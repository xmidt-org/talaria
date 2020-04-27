package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDeviceAccessCheck(t *testing.T) {
	testCases := []struct {
		config                 deviceAccessCheck
		name                   string
		fails                  bool
		expectedCheckName      string
		expectedWRPCredPath    string
		expectedDeviceCredPath string
		expectedErr            error
	}{
		{
			name: "Missing Required name",
			config: deviceAccessCheck{
				DeviceCredentialPath: "dcp",
				WRPCredentialPath:    "wcp",
				Op:                   IntersectsOp,
			},
			expectedErr: errNameRequired,
			fails:       true,
		},
		{
			name: "Missing DeviceCredentialPath",
			config: deviceAccessCheck{
				Name:              "invalid check",
				WRPCredentialPath: "wrpPath",
				Op:                IntersectsOp,
			},
			expectedErr: errDeviceCredPathRequired,
			fails:       true,
		},

		{
			name: "Nothing to compare device credential to",
			config: deviceAccessCheck{
				Name:                 "nonEmpty",
				DeviceCredentialPath: "dcp",
				Op:                   IntersectsOp,
			},
			expectedErr: errInputValueOrWRPCredPathRequired,
			fails:       true,
		},

		{
			name: "Bad operator",
			config: deviceAccessCheck{
				Name:                 "nonEmpty",
				DeviceCredentialPath: "dcp",
				InputValue:           3,
				Op:                   "bad",
			},
			expectedErr: errOpNotSupported,
			fails:       true,
		},

		{
			name: "Happpy path",
			config: deviceAccessCheck{
				Name:                 "    trimWhiteSpaces    ",
				DeviceCredentialPath: "   dcp",
				WRPCredentialPath: "wcp 		    ",
				InputValue: []string{"e0", "e1"},
				Op:         IntersectsOp,
				Inversed:   true,
			},
			expectedCheckName:      "trimWhiteSpaces",
			expectedDeviceCredPath: "dcp",
			expectedWRPCredPath:    "wcp",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			assert := assert.New(t)
			require := require.New(t)

			parsedCheck, err := parseDeviceAccessCheck(testCase.config)
			if testCase.fails {
				require.NotNil(err)
				require.Nil(parsedCheck)

				assert.Equal(testCase.expectedErr, err)
			} else {
				require.Nil(err)
				require.NotNil(parsedCheck)
				require.NotNil(parsedCheck.assertion)

				assert.Equal(testCase.expectedCheckName, parsedCheck.name)
				assert.Equal(testCase.expectedDeviceCredPath, parsedCheck.deviceCredentialPath)
				assert.Equal(testCase.expectedWRPCredPath, parsedCheck.wrpCredentialPath)
				assert.Equal(testCase.config.Inversed, parsedCheck.inversed)
				assert.Equal(testCase.config.InputValue, parsedCheck.inputValue)
			}
		})
	}
}
