// Package webui embeds Poomsae's optional browser application. API-only hosts
// can import github.com/ifigueroap/poomsae without linking these assets.
package webui

import (
	"net/http"

	"github.com/ifigueroap/poomsae/internal/web"
)

// NewEmbeddedHandler serves the browser application bundled with this module.
// Pass it as poomsae.Config.WebHandler to include it in an embedded service.
func NewEmbeddedHandler() (http.Handler, error) {
	return web.NewEmbeddedHandler()
}
