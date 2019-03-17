package main

import "html/template"
import "io"

func (s *Server) render(f string, props properties, w io.Writer) error {
	templ := template.New("main")
	templ, err := templ.Parse(f)
	if err != nil {
		return err
	}

	templ.Execute(w, props)
	return nil
}
