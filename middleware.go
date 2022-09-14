package main

import (
	"context"
	"net/http"

	// nolint:staticcheck
	"github.com/go-kit/kit/log"
	// nolint:staticcheck
	"github.com/go-kit/kit/log/level"
	"github.com/justinas/alice"
	"github.com/segmentio/ksuid"
	"github.com/xmidt-org/bascule"
	"github.com/xmidt-org/webpa-common/v2/device"

	// nolint:staticcheck
	"github.com/xmidt-org/webpa-common/v2/logging"
)

func init() {
	ksuid.SetRand(ksuid.FastRander)
}

// DeviceMetadataMiddleware is a device registration endpoint middleware
// which initializes the metadata a device carries throughout its
// connectivity lifecycle with the XMiDT cluster.
func DeviceMetadataMiddleware(getLogger func(ctx context.Context) log.Logger) alice.Constructor {
	return func(delegate http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			logger := getLogger(ctx)
			metadata := new(device.Metadata)
			metadata.SetSessionID(ksuid.New().String())

			if auth, ok := bascule.FromContext(ctx); ok {
				if tokenAttributes, ok := auth.Token.Attributes().(RawAttributes); ok {
					metadata.SetClaims(tokenAttributes.GetRawAttributes())
				} else {
					claimsMap := make(map[string]interface{})
					if partnerIDClaim, ok := auth.Token.Attributes().Get(device.PartnerIDClaimKey); ok {
						claimsMap[device.PartnerIDClaimKey] = partnerIDClaim
					}

					if trustClaim, ok := auth.Token.Attributes().Get(device.TrustClaimKey); ok {
						claimsMap[device.TrustClaimKey] = trustClaim
					}

					metadata.SetClaims(claimsMap)

				}
				if logger != nil {
					level.Info(logger).Log(logging.MessageKey(), "got claims from auth token",
						"partner-id", metadata.Claims()[device.PartnerIDClaimKey],
						"trust", metadata.Claims()[device.TrustClaimKey],
					)
				}
			}

			delegate.ServeHTTP(w, r.WithContext(device.WithDeviceMetadata(ctx, metadata)))
		})
	}
}
