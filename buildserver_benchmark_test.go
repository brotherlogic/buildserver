package main

import (
	"log"
	"testing"

	pb "github.com/brotherlogic/buildserver/proto"
	pbgbs "github.com/brotherlogic/gobuildslave/proto"
)

func BenchmarkRunBuilds(b *testing.B) {
	req := &pb.BuildRequest{Job: &pbgbs.Job{Name: "blahblahblah", GoPath: "github.com/brotherlogic/blahblahblah"}}
	d := req.Job.Name
	for i := 0; i < b.N; i++ {
		d = req.GetJob().Name
	}

	log.Printf("%v", d)
}
