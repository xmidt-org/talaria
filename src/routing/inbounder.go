package routing

import (
	"github.com/Comcast/webpa-common/device"
	"github.com/Comcast/webpa-common/logging"
	"github.com/gorilla/mux"
	"github.com/spf13/viper"
	"net/http"
)

const (
	InbounderKey    = "device.inbound"
	WRPContentType  = OutboundContentType
	JSONContentType = "application/json"

	DefaultDeviceURL     = "/device"
	DefaultDeviceListURL = "/devices"
)

type Inbounder struct {
	DeviceURL     string
	DeviceListURL string
	Logger        logging.Logger
	Manager       device.Manager
}

func NewInbounder(logger logging.Logger, manager device.Manager, v *viper.Viper) (i *Inbounder, err error) {
	i = &Inbounder{
		DeviceURL:     DefaultDeviceURL,
		DeviceListURL: DefaultDeviceListURL,
		Logger:        logger,
		Manager:       manager,
	}

	if v != nil {
		err = v.Unmarshal(i)
	}

	return
}

func (i *Inbounder) ListDevices(response http.ResponseWriter, request *http.Request) {
}

func (i *Inbounder) AcceptWRPMessage(response http.ResponseWriter, request *http.Request) {
}

func (i *Inbounder) AcceptJSONMessage(response http.ResponseWriter, request *http.Request) {
}

func (i *Inbounder) AddRoutes(router *mux.Router) {
	router.HandleFunc(i.DeviceListURL, i.ListDevices).
		Methods("GET")

	router.HandleFunc(i.DeviceURL, i.AcceptWRPMessage).
		Methods("POST").
		Headers("Content-Type", WRPContentType)

	router.HandleFunc(i.DeviceURL, i.AcceptJSONMessage).
		Methods("POST").
		Headers("Content-Type", JSONContentType)
}
