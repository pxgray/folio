// Package assets embeds the templates and static files for Folio.
package assets

import "embed"

// TemplateFS contains all HTML templates (templates/*.html).
//
//go:embed templates
var TemplateFS embed.FS

// StaticFS contains static web assets (static/*).
//
//go:embed static
var StaticFS embed.FS
