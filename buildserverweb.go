package main

import "html/template"
import "io"

func (s *Server) render(f string, props properties, w io.Writer) error {
	templ := template.New("main")
	templ.Execute(w, props)
	return nil
}
