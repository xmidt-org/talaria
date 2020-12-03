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
				metadata.SetClaims(auth.Token.Attributes().(rawAttributes).GetRawAttributes())
			}

			delegate.ServeHTTP(w, r.WithContext(device.WithDeviceMetadata(ctx, metadata)))
		})
}
