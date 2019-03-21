package main

import (
	"bytes"
	"testing"

	pb "github.com/brotherlogic/buildserver/proto"
)

func TestEasyTemplate(t *testing.T) {
	s := InitTestServer(".testtemplate")
	var buf bytes.Buffer
	err := s.render("templates/main.html", properties{Versions: []*pb.Version{
		&pb.Version{Version: "blah"},
	}}, &buf)

	if err != nil {
		t.Errorf("Rendering error: %v", err)
	}

	if len(buf.String()) == 0 {
		t.Errorf("Error in building string: %v", buf.String())
	}
}
