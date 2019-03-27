package main

import (
	"bytes"
	"testing"

	pb "github.com/brotherlogic/buildserver/proto"
)

func TestEasyTemplate(t *testing.T) {
	s := InitTestServer(".testtemplate")
	var buf bytes.Buffer
	err := s.render("templates/main.html", properties{Binaries: []string{
		"blah",
	}}, &buf)

	if err != nil {
		t.Errorf("Rendering error: %v", err)
	}

	if len(buf.String()) == 0 {
		t.Errorf("Error in building string: %v", buf.String())
	}
}

func TestEasyVersionTemplate(t *testing.T) {
	s := InitTestServer(".testtemplate")
	var buf bytes.Buffer
	err := s.renderVersion("templates/version.html", properties{Version: &pb.Version{Version: "blah", VersionDate: 1234}}, &buf)

	if err != nil {
		t.Errorf("Rendering error: %v", err)
	}

	if len(buf.String()) == 0 {
		t.Errorf("Error in building string: %v", buf.String())
	}
}

func TestEasyBinary(t *testing.T) {
	s := InitTestServer(".testtemplate")
	var buf bytes.Buffer
	err := s.renderVersion("templates/binary.html", properties{Versions: []*pb.Version{
		&pb.Version{Version: "blah", VersionDate: 1234},
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

func TestVersionTemplateFailure(t *testing.T) {
	s := InitTestServer(".testtemplatefail")
	var buf bytes.Buffer
	err := s.renderVersion("{{.broken", properties{}, &buf)

	if err == nil {
		t.Errorf("No error in processing")
	}
}

func TestBinaryTemplateFailure(t *testing.T) {
	s := InitTestServer(".testtemplatefail")
	var buf bytes.Buffer
	err := s.renderVersion("{{.broken", properties{}, &buf)

	if err == nil {
		t.Errorf("No error in processing")
	}
}
