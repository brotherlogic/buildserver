package main

import (
	"bytes"
	"testing"
)

func TestEasyTemplate(t *testing.T) {
	s := InitTestServer(".testtemplate")
	var buf bytes.Buffer
	err := s.render("templates/main.html", properties{Versions: []string{
		"blah",
	}}, &buf)

	if err != nil {
		t.Errorf("Rendering error: %v", err)
	}

	if len(buf.String()) == 0 {
		t.Errorf("Error in building string: %v", buf.String())
	}
}

func TestTemplateFailure(t *testing.T) {
	s := InitTestServer(".testtemplatefail")
	var buf bytes.Buffer
	err := s.render("{{.broken", properties{}, &buf)

	if err == nil {
		t.Errorf("No error in processing")
	}
}
