package main

import (
	"net/http"

	"github.com/xmidt-org/bascule"
	"github.com/xmidt-org/webpa-common/device"
)

//DeviceMetadataMiddleware is a device registration endpoint middleware
//constructor which initializes the metadata a device carries throughout its
//connectivity lifecycle
func DeviceMetadataMiddleware(delegate http.Handler) http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			deviceMetadata := device.NewDeviceMetadata()

			if auth, authOk := bascule.FromContext(ctx); authOk {
				deviceMetadata.SetJWTClaims(device.NewJWTClaims(auth.Token.Attributes()))
			}

			delegate.ServeHTTP(w, r.WithContext(device.WithDeviceMetadata(ctx, deviceMetadata)))
		})
}
