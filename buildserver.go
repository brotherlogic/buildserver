package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/brotherlogic/goserver"
	"github.com/brotherlogic/goserver/utils"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/brotherlogic/buildserver/proto"
	pbfc "github.com/brotherlogic/filecopier/proto"
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
	jobs              []*pbgbs.Job
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
	latestVersion     map[string]string
	blacklist         map[string]bool
	blacklistMutex    *sync.Mutex
	lockTime          time.Duration
	checkError        string
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

func (s *Server) validateBuilds(ctx context.Context) error {
	s.pathMapMutex.Lock()
	defer s.pathMapMutex.Unlock()
	for _, versionString := range s.latestVersion {
		for _, version := range s.pathMap {
			if version.Version == versionString && time.Now().Sub(time.Unix(version.LastBuildTime, 0)) > time.Hour*24 {
				s.enqueue(version.Job, true)
			}

		}
	}
	return nil
}

func (s *Server) runCheck(ctx context.Context) error {
	entries, err := utils.ResolveAll("buildserver")
	jobs := make(map[string]*pbgbs.Job)
	s.pathMapMutex.Lock()
	for _, v := range s.pathMap {
		if _, ok := jobs[v.Job.Name]; !ok {
			jobs[v.Job.Name] = v.Job
		}
	}
	s.pathMapMutex.Unlock()

	if err == nil {
		for _, entry := range entries {
			if entry.Identifier != s.Registry.Identifier {
				conn, err := s.DoDial(entry)
				if err != nil {
					return err
				}
				defer conn.Close()

				client := pb.NewBuildServiceClient(conn)
				for _, job := range jobs {
					latest, err := client.GetVersions(ctx, &pb.VersionRequest{Job: job, JustLatest: true})
					if err == nil {

						if len(latest.GetVersions()) > 0 && latest.GetVersions()[0].VersionDate > s.latestBuild[job.Name] && latest.GetVersions()[0].Version != s.latestVersion[job.Name] {
							s.blacklistMutex.Lock()
							s.blacklist[job.Name] = true
							s.blacklistMutex.Unlock()

							// Ensure blacklisted jobs get built
							s.enqueue(job, true)
						}
					}
				}
			}
		}
	} else {
		return err
	}

	return nil
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

		//Reject a full build if there's one in the queue
		for _, entry := range s.buildQueue {
			forceBuild = forceBuild && !entry.fullBuild
		}

		s.buildQueue = append(s.buildQueue, queueEntry{job: job, timeIn: time.Now(), queueSizeAtEntry: len(s.buildQueue), inFront: before, fullBuild: forceBuild})
	}
}

func (s *Server) dequeue(ctx context.Context) error {
	s.currentBuildMutex.Lock()
	defer s.currentBuildMutex.Unlock()
	if len(s.buildQueue) > 0 && s.currentBuilds < s.maxBuilds {
		if s.runBuild {
			go func() {
				job := s.buildQueue[0]
				s.currentBuilds++
				s.buildQueue = s.buildQueue[1:]
				if time.Now().Sub(job.timeIn) > time.Minute*30 {
					s.RaiseIssue(ctx, "Long Build", fmt.Sprintf("%v took %v to get to the front of the queue (%v in the queue)", job.job.Name, time.Now().Sub(job.timeIn), job.queueSizeAtEntry), false)
				}
				s.blacklistMutex.Lock()
				if len(s.blacklist) == 0 || s.blacklist[job.job.Name] {

					// Do a full build if we're blacklisted
					if s.blacklist[job.job.Name] {
						job.fullBuild = true
					}
					s.blacklistMutex.Unlock()

					_, err := s.scheduler.build(job, s.Registry.Identifier, s.latestHash[job.job.Name])
					s.checkError = fmt.Sprintf("%v", err)
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
						s.blacklistMutex.Lock()
						delete(s.blacklist, job.job.Name)
						s.blacklistMutex.Unlock()
						delete(s.buildFails, job.job.Name)
					}
					s.buildFailsMutex.Unlock()
				} else {
					s.blacklistMutex.Unlock()
				}
				s.currentBuilds--
			}()
		}
	}
	return nil
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
		s.latestVersion[jobn] = v.Version
	}
	s.pathMapMutex.Unlock()
}

func (s *Server) backgroundBuilder(ctx context.Context) error {
	oldestJob := ""
	oldest := time.Now().Unix()
	for key, val := range s.latestBuild {
		if val < oldest {
			oldest = val
			oldestJob = key
		}
	}

	s.enqueue(&pbgbs.Job{Name: oldestJob, GoPath: "github.com/brotherlogic/" + oldestJob}, false)
	return nil
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
		make([]*pbgbs.Job, 0),
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
		make(map[string]string),
		make(map[string]bool),
		&sync.Mutex{},
		0,
		"",
	}

	s.scheduler.log = s.log
	s.scheduler.load = s.load

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
	s.preloadInfo()
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

	memoryCrashes := make(map[string]int)
	for _, v := range s.pathMap {
		mCrash := 0
		for _, c := range v.Crashes {
			if c.CrashType == pb.Crash_MEMORY {
				mCrash++
			}
		}

		if mCrash > 0 {
			if _, ok := memoryCrashes[v.Job.Name]; !ok {
				memoryCrashes[v.Job.Name] = 0
			}
			memoryCrashes[v.Job.Name] += mCrash
		}
	}

	s.blacklistMutex.Lock()
	defer s.blacklistMutex.Unlock()
	return []*pbg.State{
		&pbg.State{Key: "check_error", Text: s.checkError},
		&pbg.State{Key: "latest_versions", Value: int64(len(s.latestVersion))},
		&pbg.State{Key: "build_queue_length", Value: int64(len(s.buildQueue))},
		&pbg.State{Key: "lock_time", TimeDuration: s.lockTime.Nanoseconds()},
		&pbg.State{Key: "versions", Value: int64(len(s.pathMap))},
		&pbg.State{Key: "memory", Text: fmt.Sprintf("%v", memoryCrashes)},
		&pbg.State{Key: "blacklist", Text: fmt.Sprintf("%v", s.blacklist)},
		&pbg.State{Key: "enabled", Text: fmt.Sprintf("%v", s.runBuild)},
		&pbg.State{Key: "buildc", Value: int64(s.buildRequest)},
		&pbg.State{Key: "concurrent_builds", Value: int64(s.currentBuilds)},
		&pbg.State{Key: "crashes", Value: s.crashes},
		&pbg.State{Key: "paths_read", Value: int64(len(s.pathMap))},
		&pbg.State{Key: "current_build", Text: s.scheduler.cbuild},
		&pbg.State{Key: "build_fails", Text: fmt.Sprintf("%v", s.buildFails)},
		&pbg.State{Key: "hashses_stored", Value: int64(counts)},
		&pbg.State{Key: "scheduled_runs", Value: s.scheduler.runs},
		&pbg.State{Key: "command_runs", Value: s.scheduler.cRuns},
		&pbg.State{Key: "command_finishes", Value: s.scheduler.cFins},
	}
}

type properties struct {
	Binaries []string
	Versions []*pb.Version
	Version  *pb.Version
}

func (s *Server) deliver(w http.ResponseWriter, r *http.Request) {
	binaries := []string{}
	for _, v := range s.pathMap {
		found := false
		for _, ver := range binaries {
			if ver == v.Job.Name {
				found = true
			}
		}
		if !found {
			binaries = append(binaries, v.Job.Name)
		}
	}
	data, err := Asset("templates/main.html")
	if err != nil {
		fmt.Fprintf(w, fmt.Sprintf("Error: %v", err))
		return
	}
	err = s.render(string(data), properties{Binaries: binaries}, w)
	if err != nil {
		s.Log(fmt.Sprintf("Error writing: %v", err))
	}
}

func (s *Server) deliverVersion(w http.ResponseWriter, r *http.Request) {
	var version *pb.Version
	ver, ok := r.URL.Query()["version"]

	if !ok {
		return
	}

	for _, v := range s.pathMap {
		if v.Version == ver[0] {
			version = v
		}
	}

	data, err := Asset("templates/version.html")
	if err != nil {
		fmt.Fprintf(w, fmt.Sprintf("Error: %v", err))
		return
	}
	err = s.renderVersion(string(data), properties{Version: version}, w)
	if err != nil {
		s.Log(fmt.Sprintf("Error writing: %v", err))
	}
}

func (s *Server) deliverBinary(w http.ResponseWriter, r *http.Request) {
	versions := []*pb.Version{}
	binary, ok := r.URL.Query()["binary"]

	if !ok {
		return
	}

	for _, v := range s.pathMap {
		if v.Job.Name == binary[0] {
			versions = append(versions, v)
		}
	}
	data, err := Asset("templates/binary.html")
	if err != nil {
		fmt.Fprintf(w, fmt.Sprintf("Error: %v", err))
		return
	}
	err = s.renderBinary(string(data), properties{Versions: versions}, w)
	if err != nil {
		s.Log(fmt.Sprintf("Error writing: %v", err))
	}
}

func (s *Server) serveUp(port int32) {
	http.HandleFunc("/", s.deliver)
	http.HandleFunc("/version", s.deliverVersion)
	http.HandleFunc("/binary", s.deliverBinary)
	err := http.ListenAndServe(fmt.Sprintf(":%v", port), nil)
	if err != nil {
		panic(err)
	}
}

func (s *Server) aligner(ctx context.Context) error {
	if !s.Registry.Master {
		s.Log(fmt.Sprintf("Running alignment on %v jobs", len(s.jobs)))
		for _, job := range s.jobs {
			s.Log(fmt.Sprintf("Aligning %v", job.Name))
			entries, err := utils.ResolveAll("buildserver")
			if err != nil {
				return err
			}

			for _, e := range entries {
				conn, err := s.DoDial(e)
				if err != nil {
					return err
				}
				defer conn.Close()

				client := pb.NewBuildServiceClient(conn)
				latest, err := client.GetVersions(ctx, &pb.VersionRequest{Job: job, JustLatest: true})
				if err != nil {
					return err
				}

				if len(latest.GetVersions()) > 0 &&
					latest.GetVersions()[0].VersionDate > s.latestBuild[job.Name] &&
					latest.GetVersions()[0].Version != s.latestVersion[job.Name] {
					cconn, err := s.DialMaster("filecopier")
					if err != nil {
						return err
					}
					defer cconn.Close()

					cclient := pbfc.NewFileCopierServiceClient(cconn)
					cclient.QueueCopy(ctx, &pbfc.CopyRequest{
						InputFile:    latest.GetVersions()[0].Path,
						InputServer:  latest.GetVersions()[0].Server,
						OutputServer: s.Registry.Identifier,
						OutputFile:   latest.GetVersions()[0].Path,
					})
					cclient.QueueCopy(ctx, &pbfc.CopyRequest{
						InputFile:    latest.GetVersions()[0].Path + ".version",
						InputServer:  latest.GetVersions()[0].Server,
						OutputServer: s.Registry.Identifier,
						OutputFile:   latest.GetVersions()[0].Path + ".version",
					})

				}
			}
		}
	}

	s.preloadInfo()

	return nil
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
	//server.GoServer.KSclient = *keystoreclient.GetClient(server.DialMaster)
	server.PrepServer()
	server.Register = server

	server.RegisterServer("buildserver", false)

	go server.serveUp(server.Registry.Port - 1)

	server.RegisterRepeatingTask(server.backgroundBuilder, "background_builder", time.Minute*5)
	server.RegisterRepeatingTask(server.runCheck, "checker", time.Minute*5)
	server.RegisterRepeatingTaskNonMaster(server.dequeue, "dequeue", time.Second)
	server.RegisterRepeatingTaskNonMaster(server.aligner, "aligner", time.Minute)
	server.RegisterRepeatingTask(server.validateBuilds, "validateBuilds", time.Minute)

	fmt.Printf("%v\n", server.Serve())
}
