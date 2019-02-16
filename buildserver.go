package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/brotherlogic/goserver"
	"github.com/brotherlogic/goserver/utils"
	"github.com/brotherlogic/keystore/client"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/brotherlogic/buildserver/proto"
	pbgbs "github.com/brotherlogic/gobuildslave/proto"
	pbg "github.com/brotherlogic/goserver/proto"
)

type queueEntry struct {
	job              *pbgbs.Job
	timeIn           time.Time
	fullBuild        bool
	queueSizeAtEntry int
	buildTime        time.Duration
	inFront          []queueEntry
	pastGithubHash   string
}

//Server main server type
type Server struct {
	*goserver.GoServer
	scheduler         *Scheduler
	builds            map[string]time.Time
	buildsMutex       *sync.Mutex
	dir               string
	lister            lister
	jobs              map[string]*pbgbs.Job
	jobsMutex         *sync.Mutex
	buildRequest      int
	runBuild          bool
	currentBuilds     int
	currentBuildMutex *sync.Mutex
	buildQueue        []queueEntry
	nobuild           []string
	pathMap           map[string]*pb.Version
	pathMapMutex      *sync.Mutex
	crashes           int64
	maxBuilds         int
	buildFails        map[string]int
	buildFailsMutex   *sync.Mutex
	latestHash        map[string]string
	latestBuild       map[string]int64
	latestDate        map[string]time.Time
	blacklist         map[string]bool
	blacklistMutex    *sync.Mutex
}

type fileDetails struct {
	path string
	date int64
}

type lister interface {
	listFiles(job *pbgbs.Job) ([]fileDetails, error)
}

type prodLister struct {
	dir  string
	fail bool
}

func (s *Server) runCheck(ctx context.Context) {
	entries, err := utils.ResolveAll("buildserver")
	if err == nil {
		for _, entry := range entries {
			if entry.Identifier != s.Registry.Identifier {
				conn, err := grpc.Dial(entry.Ip+":"+strconv.Itoa(int(entry.Port)), grpc.WithInsecure())
				defer conn.Close()

				if err != nil {
					log.Fatalf("Unable to dial: %v", err)
				}

				client := pb.NewBuildServiceClient(conn)

				s.jobsMutex.Lock()
				for _, job := range s.jobs {
					latest, err := client.GetVersions(ctx, &pb.VersionRequest{Job: job, JustLatest: true})
					if err == nil {

						if len(latest.GetVersions()) > 0 && latest.GetVersions()[0].VersionDate > s.latestBuild[job.Name] && latest.GetVersions()[0].Version != s.latestHash[job.Name] {
							s.blacklist[job.Name] = true
						}
					}
				}
				s.jobsMutex.Unlock()
			}
		}
	}
}

func (s *Server) enqueue(job *pbgbs.Job, force bool) {
	//Only enqueue if the job isn't already there
	found := false
	for _, j := range s.buildQueue {
		if j.job.Name == job.Name {
			found = true
		}
	}

	if !found {
		s.buildsMutex.Lock()
		s.builds[job.Name] = time.Now()
		s.buildsMutex.Unlock()
		before := []queueEntry{}
		for _, ent := range s.buildQueue {
			before = append(before, ent)
		}

		forceBuild := force
		if val, ok := s.latestDate[job.Name]; ok {
			if time.Now().Sub(val) > time.Hour*24 {
				forceBuild = true
			}
		}
		s.buildQueue = append(s.buildQueue, queueEntry{job: job, timeIn: time.Now(), queueSizeAtEntry: len(s.buildQueue), inFront: before, fullBuild: forceBuild})
	}
}

func (s *Server) dequeue(ctx context.Context) {
	if len(s.buildQueue) > 0 && s.currentBuilds < s.maxBuilds {
		if s.runBuild {
			go func() {
				job := s.buildQueue[0]
				s.currentBuilds++
				s.buildQueue = s.buildQueue[1:]
				if time.Now().Sub(job.timeIn) > time.Minute*10 {
					s.RaiseIssue(ctx, "Long Build", fmt.Sprintf("%v took %v to get to the front of the queue (%v in the queue) %v", job.job.Name, time.Now().Sub(job.timeIn), job.queueSizeAtEntry, job.inFront[0]), false)
				}
				_, err := s.scheduler.build(job, s.Registry.Identifier, s.latestHash[job.job.Name])
				s.buildFailsMutex.Lock()
				if err != nil {
					e, ok := status.FromError(err)
					if !ok || e.Code() != codes.AlreadyExists {
						s.buildFails[job.job.Name]++
						if s.buildFails[job.job.Name] > 3 {
							s.RaiseIssue(ctx, "Build Failure", fmt.Sprintf("Build failed for %v: %v running on %v", job.job.Name, err, s.Registry.Identifier), false)
						}
					}
				} else {
					delete(s.blacklist, job.job.Name)
					delete(s.buildFails, job.job.Name)
				}
				s.buildFailsMutex.Unlock()
				s.currentBuilds--
			}()
		}
	}
}

func (s *Server) drainQueue(ctx context.Context) {
	for len(s.buildQueue) > 0 || s.currentBuilds > 0 {
		s.dequeue(ctx)
		time.Sleep(time.Second)
	}
}

func (s *Server) load(v *pb.Version) {
	s.pathMapMutex.Lock()
	s.pathMap[v.Path] = v
	jobn := v.Job.Name
	if v.VersionDate > s.latestBuild[jobn] {
		s.latestBuild[jobn] = v.VersionDate
		s.latestHash[jobn] = v.GithubHash
		s.latestDate[jobn] = time.Unix(v.VersionDate, 0)
	}
	s.pathMapMutex.Unlock()
}

func (s *Server) backgroundBuilder(ctx context.Context) {
	s.jobsMutex.Lock()
	for _, j := range s.jobs {
		s.enqueue(j, false)
	}
	s.jobsMutex.Unlock()
}

func (p *prodLister) listFiles(job *pbgbs.Job) ([]fileDetails, error) {
	vals := make([]fileDetails, 0)
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
		fpath := p.dir + "/builds/" + job.GoPath + "/" + f.Name()
		info, _ := os.Stat(fpath)
		vals = append(vals, fileDetails{
			path: fpath,
			date: info.ModTime().Unix(),
		})
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
			"md5sum",
			nil,
			&sync.Mutex{},
			make(map[string]time.Time),
			"",
			time.Minute * 2,
			int64(0),
			int64(0),
			int64(0),
		},
		make(map[string]time.Time),
		&sync.Mutex{},
		"/media/scratch/buildserver",
		&prodLister{dir: "/media/scratch/buildserver"},
		make(map[string]*pbgbs.Job),
		&sync.Mutex{},
		0,
		true,
		0,
		&sync.Mutex{},
		[]queueEntry{},
		[]string{"led"},
		make(map[string]*pb.Version),
		&sync.Mutex{},
		int64(0),
		2,
		make(map[string]int),
		&sync.Mutex{},
		make(map[string]string),
		make(map[string]int64),
		make(map[string]time.Time),
		make(map[string]bool),
		&sync.Mutex{},
	}

	s.scheduler.log = s.log
	s.scheduler.load = s.load

	s.blacklist["recordwants"] = true

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

// Shutdown the server
func (s *Server) Shutdown(ctx context.Context) error {
	return nil
}

// Mote promotes/demotes this server
func (s *Server) Mote(ctx context.Context, master bool) error {
	return nil
}

// GetState gets the state of the server
func (s *Server) GetState() []*pbg.State {
	s.pathMapMutex.Lock()
	defer s.pathMapMutex.Unlock()
	s.buildFailsMutex.Lock()
	defer s.buildFailsMutex.Unlock()

	counts := 0
	for _, hash := range s.latestHash {
		if len(hash) > 0 {
			counts++
		}
	}

	s.blacklistMutex.Lock()
	defer s.blacklistMutex.Unlock()
	return []*pbg.State{
		&pbg.State{Key: "blacklist", Text: fmt.Sprintf("%v", s.blacklist)},
		&pbg.State{Key: "enabled", Text: fmt.Sprintf("%v", s.runBuild)},
		&pbg.State{Key: "buildc", Value: int64(s.buildRequest)},
		&pbg.State{Key: "concurrent_builds", Value: int64(s.currentBuilds)},
		&pbg.State{Key: "build_queue_length", Value: int64(len(s.buildQueue))},
		&pbg.State{Key: "crashes", Value: s.crashes},
		&pbg.State{Key: "paths_read", Value: int64(len(s.pathMap))},
		&pbg.State{Key: "current_build", Text: s.scheduler.cbuild},
		&pbg.State{Key: "build_fails", Text: fmt.Sprintf("%v", s.buildFails)},
		&pbg.State{Key: "hashses_stored", Value: int64(counts)},
		&pbg.State{Key: "scheduled_runs", Value: s.scheduler.runs},
		&pbg.State{Key: "command_runs", Value: s.scheduler.cRuns},
		&pbg.State{Key: "command_finishes", Value: s.scheduler.cFins},
		&pbg.State{Key: "profiling", Value: int64(2)},
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

	server.RegisterRepeatingTask(server.backgroundBuilder, "background_builder", time.Minute*5)
	server.RegisterRepeatingTask(server.runCheck, "checker", time.Minute*1)
	server.RegisterRepeatingTaskNonMaster(server.dequeue, "dequeue", time.Second)

	server.preloadInfo()

	fmt.Printf("%v\n", server.Serve())
}
