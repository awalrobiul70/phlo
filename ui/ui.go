package ui

import "embed"

// Files is the dashboard served by the Phlo process.
//
//go:embed index.html app.css app.js logo.svg panel.html panel.css panel.js
var Files embed.FS
