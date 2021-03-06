package marketplacesd

import (
	log "github.com/Sirupsen/logrus"
	"github.com/virtengine/vertice/marketplaces"
)

type Handler struct {
	d            *Config
	EventChannel chan bool
}

func NewHandler(c *Config) *Handler {
	return &Handler{d: c}

}

func (h *Handler) serveNSQ(r *marketplaces.ReqOpts) error {
	req, err := r.ParseRequest()
	if err != nil {
		log.Errorf("Error parsing request : %s  -  %s  : %s", r.Category, r.Action, err)
		return err
	}
	return req.Process(r.Action)
}
