package main

import (
	"net/http"

	"github.com/segmentio/ksuid"
	"github.com/xmidt-org/bascule"
	"github.com/xmidt-org/webpa-common/device"
)

func init() {
	ksuid.SetRand(ksuid.FastRander)
}

// DeviceMetadataMiddleware is a device registration endpoint middleware
// which initializes the metadata a device carries throughout its
// connectivity lifecycle with the XMiDT cluster.
func DeviceMetadataMiddleware(delegate http.Handler) http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
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
			}

			delegate.ServeHTTP(w, r.WithContext(device.WithDeviceMetadata(ctx, metadata)))
		})
}
