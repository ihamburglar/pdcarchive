package web

import (
	"embed"
	"html/template"
	"io/fs"
)

//go:embed templates/*
var templateFS embed.FS

func LoadTemplates() (*template.Template, error) {
	sub, err := fs.Sub(templateFS, "templates")
	if err != nil {
		return nil, err
	}
	return template.ParseFS(sub, "*.html")
}
