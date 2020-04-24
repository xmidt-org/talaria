package main

import (
	"net/http"

	"github.com/xmidt-org/bascule"
	"github.com/xmidt-org/webpa-common/device"
)

// DeviceMetadataMiddleware is a device registration endpoint middleware
// which initializes the metadata a device carries throughout its
// connectivity lifecycle with the XMiDT cluster.
func DeviceMetadataMiddleware(delegate http.Handler) http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			deviceMetadata := device.NewDeviceMetadata()

			if auth, authOk := bascule.FromContext(ctx); authOk {
				deviceMetadata.SetJWTClaims(device.JWTClaims(auth.Token.Attributes().FullView()))
			}

			delegate.ServeHTTP(w, r.WithContext(device.WithDeviceMetadata(ctx, deviceMetadata)))
		})
}
