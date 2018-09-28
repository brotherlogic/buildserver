package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"sync"
	"time"

	"github.com/brotherlogic/goserver"
	"github.com/brotherlogic/keystore/client"
	"golang.org/x/net/context"
	"google.golang.org/grpc"

	pb "github.com/brotherlogic/buildserver/proto"
	pbgbs "github.com/brotherlogic/gobuildslave/proto"
	pbg "github.com/brotherlogic/goserver/proto"
)

//Server main server type
type Server struct {
	*goserver.GoServer
	scheduler    *Scheduler
	builds       map[string]time.Time
	dir          string
	lister       lister
	jobs         map[string]*pbgbs.Job
	buildRequest int
}

type lister interface {
	listFiles(job *pbgbs.Job) ([]string, error)
}

type prodLister struct {
	dir  string
	fail bool
}

func (s *Server) backgroundBuilder(ctx context.Context) {
	for _, j := range s.jobs {
		go s.scheduler.build(j)
	}
}

func (p *prodLister) listFiles(job *pbgbs.Job) ([]string, error) {
	vals := make([]string, 0)
	if p.fail {
		return vals, fmt.Errorf("Built to fail")
	}

	// If there's no such directory, return no versions
	directory := p.dir + "/builds/" + job.GoPath + "/"
	_, err := os.Stat(directory)
	if err != nil && os.IsNotExist(err) {
		return vals, nil
	}

	files, err := ioutil.ReadDir(p.dir + "/builds/" + job.GoPath + "/")
	if err != nil {
		return vals, err
	}

	for _, f := range files {
		vals = append(vals, p.dir+"/builds/"+job.GoPath+"/"+f.Name())
	}

	return vals, nil
}

func (s *Server) log(st string) {
	s.Log(st)
}

// Init builds the server
func Init() *Server {
	s := &Server{
		&goserver.GoServer{},
		&Scheduler{
			"/media/scratch/buildserver",
			&sync.Mutex{},
			make(map[string]*sync.Mutex),
			nil,
		},
		make(map[string]time.Time),
		"/media/scratch/buildserver",
		&prodLister{dir: "/media/scratch/buildserver"},
		make(map[string]*pbgbs.Job),
		0,
	}

	s.scheduler.log = s.log

	return s
}

// DoRegister does RPC registration
func (s *Server) DoRegister(server *grpc.Server) {
	pb.RegisterBuildServiceServer(server, s)
}

// ReportHealth alerts if we're not healthy
func (s *Server) ReportHealth() bool {
	return true
}

// Mote promotes/demotes this server
func (s *Server) Mote(ctx context.Context, master bool) error {
	return nil
}

// GetState gets the state of the server
func (s *Server) GetState() []*pbg.State {
	return []*pbg.State{
		&pbg.State{Key: "builds", Text: fmt.Sprintf("%v", s.builds)},
		&pbg.State{Key: "buildc", Value: int64(s.buildRequest)},
	}
}

func main() {
	var quiet = flag.Bool("quiet", false, "Show all output")
	flag.Parse()

	//Turn off logging
	if *quiet {
		log.SetFlags(0)
		log.SetOutput(ioutil.Discard)
	}
	server := Init()
	server.GoServer.KSclient = *keystoreclient.GetClient(server.GetIP)
	server.PrepServer()
	server.Register = server

	server.RegisterServer("buildserver", false)

	server.RegisterRepeatingTask(server.backgroundBuilder, "background_builder", time.Hour)

	fmt.Printf("%v\n", server.Serve())
}
