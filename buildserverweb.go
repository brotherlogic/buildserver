package main

import "io"
import "html/template"

func (s *Server) render(f string, props properties, w io.Writer) error {
	templ := template.New("main")
	templ, err := templ.Parse(f)
	if err != nil {
		return err
	}
	templ.Execute(w, props)
	return nil
}

func (s *Server) renderVersion(f string, props properties, w io.Writer) error {
	templ := template.New("version")
	templ, err := templ.Parse(f)
	if err != nil {
		return err
	}
	templ.Execute(w, props)
	return nil
}
