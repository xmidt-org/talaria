package main

import (
	"net/http"

	"github.com/xmidt-org/bascule"
	"github.com/xmidt-org/webpa-common/device"
)

func DeviceMetadataDecorator(delegate http.Handler) http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			deviceMetadata := device.DefaultDeviceMetadata()

			if auth, authOk := bascule.FromContext(ctx); authOk {
				deviceMetadata.SetJWTClaims(device.NewJWTClaims(auth.Token.Attributes()))
			}

			delegate.ServeHTTP(w, r.WithContext(device.WithDeviceMetadata(deviceMetadata, ctx)))
		})
}
