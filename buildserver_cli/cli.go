package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/brotherlogic/goserver/utils"
	"google.golang.org/grpc"

	pb "github.com/brotherlogic/buildserver/proto"
	pbgbs "github.com/brotherlogic/gobuildslave/proto"
	pbgs "github.com/brotherlogic/goserver/proto"
	pbt "github.com/brotherlogic/tracer/proto"

	//Needed to pull in gzip encoding init
	_ "google.golang.org/grpc/encoding/gzip"
)

func main() {
	ctx, cancel := utils.BuildContext("buildserver-"+os.Args[1], "buildserver", pbgs.ContextType_MEDIUM)
	defer cancel()

	host, port, err := utils.Resolve("buildserver")
	if err != nil {
		log.Fatalf("Unable to reach organiser: %v", err)
	}
	conn, err := grpc.Dial(host+":"+strconv.Itoa(int(port)), grpc.WithInsecure())
	defer conn.Close()

	if err != nil {
		log.Fatalf("Unable to dial: %v", err)
	}

	client := pb.NewBuildServiceClient(conn)

	switch os.Args[1] {
	case "build":
		res, err := client.Build(ctx, &pb.BuildRequest{Job: &pbgbs.Job{Name: os.Args[2], GoPath: "github.com/brotherlogic/" + os.Args[2]}})
		if err != nil {
			log.Fatalf("Error on build: %v", err)
		}

		fmt.Printf("Response: %v\n", res)
	case "crash":
		file, err := ioutil.ReadFile(os.Args[4])
		if err != nil {
			log.Fatalf("Error reading file: %v", err)
		}

		_, err = client.ReportCrash(ctx, &pb.CrashRequest{Job: &pbgbs.Job{Name: os.Args[2]}, Version: os.Args[3], Crash: &pb.Crash{ErrorMessage: string(file)}})
		if err != nil {
			log.Fatalf("Error reporting: %v", err)
		}
	}
	utils.SendTrace(ctx, "builserver-"+os.Args[1], time.Now(), pbt.Milestone_END, "buildserver")
}
