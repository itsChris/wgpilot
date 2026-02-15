package wgpilot

import "embed"

// FrontendDist embeds the built frontend SPA. The directory is populated by
// running "npm run build" inside the frontend/ directory before "go build".
//
//go:embed all:frontend/dist
var FrontendDist embed.FS
