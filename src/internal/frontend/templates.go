package frontend

import (
	"embed"
	"html/template"
	"path"
)

//go:embed templates/*
var TemplatesFS embed.FS

// BuildTemplates parses partials and pages and returns a *template.Template.
// Call this once at startup and pass the result into your Handler.
func BuildTemplates() (*template.Template, error) {
	t := template.New("app").Funcs(template.FuncMap{
		// add helpers here if needed, e.g. URL building, formatters
	})
	// parse partials first so pages can use them
	if _, err := t.ParseFS(TemplatesFS, "templates/partials/*.html"); err != nil {
		return nil, err
	}
	// parse all page templates
	if _, err := t.ParseFS(TemplatesFS, "templates/*.html"); err != nil {
		return nil, err
	}
	return t, nil
}

// helper to get template name by file (optional)
func TemplateName(f string) string {
	return path.Base(f)
}
