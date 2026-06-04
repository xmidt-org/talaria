// SPDX-FileCopyrightText: 2017 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"net/http"

	"go.uber.org/zap"

	"github.com/justinas/alice"
	"github.com/segmentio/ksuid"
	"github.com/xmidt-org/bascule"
	"github.com/xmidt-org/webpa-common/v2/device"
	// nolint:staticcheck
)

func init() {
	ksuid.SetRand(ksuid.FastRander)
}

// DeviceMetadataMiddleware is a device registration endpoint middleware
// which initializes the metadata a device carries throughout its
// connectivity lifecycle with the XMiDT cluster.
func DeviceMetadataMiddleware(getLogger func(ctx context.Context) *zap.Logger) alice.Constructor {
	return func(delegate http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			logger := getLogger(ctx)
			metadata := new(device.Metadata)
			metadata.SetSessionID(ksuid.New().String())

			if token, ok := bascule.Get(ctx); ok {
				if tokenAttributes, ok := token.(RawAttributes); ok {
					metadata.SetClaims(tokenAttributes.GetRawAttributes())
				} else if accessor, ok := token.(bascule.AttributesAccessor); ok {
					claimsMap := make(map[string]interface{})
					if partnerIDClaim, ok := accessor.Get(device.PartnerIDClaimKey); ok {
						claimsMap[device.PartnerIDClaimKey] = partnerIDClaim
					}

					if trustClaim, ok := accessor.Get(device.TrustClaimKey); ok {
						claimsMap[device.TrustClaimKey] = trustClaim
					}

					if accountIDClaim, ok := accessor.Get(device.AccountIDClaimKey); ok {
						claimsMap[device.AccountIDClaimKey] = accountIDClaim
					}

					metadata.SetClaims(claimsMap)

				}
				if logger != nil {
					logger.Info("got claims from auth token", zap.Any("partner-id", metadata.Claims()[device.PartnerIDClaimKey]), zap.Any("trust", metadata.Claims()[device.TrustClaimKey]))
				}
			}

			delegate.ServeHTTP(w, r.WithContext(device.WithDeviceMetadata(ctx, metadata)))
		})
	}
}
