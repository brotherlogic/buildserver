package main

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	pbgbs "github.com/brotherlogic/gobuildslave/proto"
	pid "github.com/struCoder/pidusage"
)

func BenchmarkRunBuilds(b *testing.B) {
	wd, _ := os.Getwd()
	f := ".testbenchmark"
	buildserver := CloneTestServer(f, true)
	buildserver.scheduler.waitTime = 0

	for i := 0; i < b.N; i++ {
		//Run 10 builds
		for j := 0; j < 5; j++ {
			for buildserver.currentBuilds > 0 {
				time.Sleep(time.Millisecond)
			}
			buildserver.enqueue(&pbgbs.Job{GoPath: "github.com/brotherlogic/buildserver", Name: "buildserver"}, false)
			buildserver.dequeue(context.Background())
			os.RemoveAll(wd + "/" + f)

			v, _ := pid.GetStat(os.Getpid())
			log.Printf("MEM = %v", v.Memory)
		}
	}

	log.Printf("DONINGTON")
	buildserver.enqueue(&pbgbs.Job{GoPath: "github.com/brotherlogic/monitor", Name: "monitor"}, false)
	buildserver.dequeue(context.Background())
	os.RemoveAll(wd + "/" + f)

	v, _ := pid.GetStat(os.Getpid())
	log.Printf("MEM = %v", v.Memory)

}
