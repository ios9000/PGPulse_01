package web

import "embed"

// DistFS contains the built frontend assets from web/dist/.
// To build: cd web && npm run build
//
//go:embed all:dist
var DistFS embed.FS
