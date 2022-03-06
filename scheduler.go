package main

import (
	"bufio"
	"crypto/md5"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/brotherlogic/buildserver/proto"
	pbgbs "github.com/brotherlogic/gobuildslave/proto"
)

// Scheduler for doing builds
type Scheduler struct {
	dir            string
	masterMutex    *sync.Mutex
	mMap           map[string]*sync.Mutex
	log            func(s string)
	md5command     string
	load           func(v *pb.Version)
	lastBuildMutex *sync.Mutex
	lastBuild      map[string]time.Time
	cbuild         string
	waitTime       time.Duration
	runs           int64
	cRuns          int64
	cFins          int64
}

type rCommand struct {
	command   *exec.Cmd
	output    string
	erroutput string
	startTime int64
	endTime   int64
	err       error
}

func (s *Scheduler) saveVersionInfo(j *pbgbs.Job, path string, server string, githubHash string) *pb.Version {
	f, err := os.Stat(path)
	if err == nil {
		ver := &pb.Version{
			Job:           j,
			Version:       getVersion(path),
			Path:          path,
			Server:        server,
			VersionDate:   f.ModTime().Unix(),
			GithubHash:    githubHash,
			LastBuildTime: time.Now().Unix(),
		}

		s.saveVersionFile(ver)
		return ver
	}

	return nil
}

func (s *Scheduler) saveVersionFile(v *pb.Version) {
	nfile := v.Path + ".version"
	data, _ := proto.Marshal(v)
	ioutil.WriteFile(nfile, data, 0644)
	s.load(v)
}

func (s *Scheduler) build(queEnt queueEntry, server string, latestHash string) (string, *pb.Version, error) {
	s.cbuild = fmt.Sprintf("%v @ %v", queEnt.job.Name, time.Now())
	s.lastBuildMutex.Lock()
	if val, ok := s.lastBuild[queEnt.job.Name]; ok && time.Now().Sub(val) < s.waitTime {
		s.lastBuildMutex.Unlock()
		return "", nil, status.Error(codes.AlreadyExists, fmt.Sprintf("Skipping build for %v since we have a recent one", queEnt.job.Name))
	}
	s.lastBuild[queEnt.job.Name] = time.Now()
	s.lastBuildMutex.Unlock()

	if queEnt.job.Name == "" {
		return "", nil, fmt.Errorf("Job is not specified correctly (has no name)")
	}

	// Prep the mutex
	s.masterMutex.Lock()
	if _, ok := s.mMap[queEnt.job.Name]; !ok {
		s.mMap[queEnt.job.Name] = &sync.Mutex{}
	}
	s.masterMutex.Unlock()

	// Lock the job for the duration of the build
	s.mMap[queEnt.job.Name].Lock()
	defer s.mMap[queEnt.job.Name].Unlock()

	//Refresh the project
	if !queEnt.fullBuild {
		fetchCommand := &rCommand{command: exec.Command("git", "-C", s.dir+"/src/"+queEnt.job.GoPath, "fetch", "-p")}
		s.runAndWait(fetchCommand)
		mergeCommand := &rCommand{command: exec.Command("git", "-C", s.dir+"/src/"+queEnt.job.GoPath, "merge", "origin/master")}
		s.runAndWait(mergeCommand)
		mergeCommand = &rCommand{command: exec.Command("git", "-C", s.dir+"/src/"+queEnt.job.GoPath, "merge", "origin/main")}
		s.runAndWait(mergeCommand)

		readHash := ""
		data, err := ioutil.ReadFile(s.dir + "/src/" + queEnt.job.GoPath + "/.git/refs/heads/master")
		if err == nil {
			readHash = strings.TrimSpace(string(data))
		} else {
			data, err = ioutil.ReadFile(s.dir + "/src/" + queEnt.job.GoPath + "/.git/refs/heads/main")
			if err == nil {
				readHash = strings.TrimSpace(string(data))
			}
		}

		if len(latestHash) > 0 && readHash == latestHash {
			return "", nil, status.Error(codes.AlreadyExists, fmt.Sprintf("Skipping build for %v since we have a recent hash %v from %v", queEnt.job.Name, latestHash, s.dir+"/src/"+queEnt.job.GoPath))
		}
	}

	s.log(fmt.Sprintf("BUILDING %v {%v} (%v)", queEnt.job.Name, time.Now().Sub(queEnt.timeIn), queEnt.fullBuild))

	buildCommand := &rCommand{command: exec.Command("go", "install", fmt.Sprintf("%v@latest", queEnt.job.GoPath))}
	s.runAndWait(buildCommand)
	s.log(fmt.Sprintf("Have Ran the build (%v): %v and %v -> %+v, %+v", queEnt.job.GoPath, buildCommand.output, buildCommand.erroutput, queEnt, queEnt.job))

	builtHash := ""
	data, err := ioutil.ReadFile(s.dir + "/src/" + queEnt.job.GoPath + "/.git/refs/heads/master")
	if err != nil {
		data, _ = ioutil.ReadFile(s.dir + "/src/" + queEnt.job.GoPath + "/.git/refs/heads/main")
	}
	builtHash = strings.TrimSpace(string(data))

	// If the build has failed, there will be no file output
	if _, err := os.Stat(s.dir + "/bin/" + queEnt.job.Name); os.IsNotExist(err) && (len(buildCommand.output) > 0 || len(buildCommand.erroutput) > 0) {
		outy, err := exec.Command("pwd").Output()
		return "", nil, fmt.Errorf("Build failed: %v and %v -> %v and extra %v and %v", buildCommand.output, buildCommand.erroutput, buildCommand.err, string(outy), err)
	}

	data, _ = ioutil.ReadFile(s.dir + "/bin/" + queEnt.job.Name)
	hash := fmt.Sprintf("%x", md5.Sum(data))

	os.MkdirAll(s.dir+"/builds/"+queEnt.job.GoPath, 0755)
	copyCommand := &rCommand{command: exec.Command("mv", s.dir+"/bin/"+queEnt.job.Name, s.dir+"/builds/"+queEnt.job.GoPath+"/"+queEnt.job.Name+"-"+hash)}
	s.runAndWait(copyCommand)

	version := s.saveVersionInfo(queEnt.job, s.dir+"/builds/"+queEnt.job.GoPath+"/"+queEnt.job.Name+"-"+hash, server, builtHash)

	return hash, version, nil
}

func (s *Scheduler) runAndWait(c *rCommand) {
	s.cRuns++
	c.err = s.run(c)
	if c.err == nil {
		for c.endTime == 0 {
			time.Sleep(time.Second)
		}
	}
	s.cFins++
}

func (s *Scheduler) run(c *rCommand) error {
	s.runs++
	env := os.Environ()
	gpath := s.dir
	c.command.Path = strings.Replace(c.command.Path, "$GOPATH", gpath, -1)
	for i := range c.command.Args {
		c.command.Args[i] = strings.Replace(c.command.Args[i], "$GOPATH", gpath, -1)
	}
	path := fmt.Sprintf("GOPATH=" + s.dir)
	pathbin := fmt.Sprintf("GOBIN=" + s.dir + "/bin")
	found := false
	for i, blah := range env {
		if strings.HasPrefix(blah, "GOPATH") {
			env[i] = path
			found = true
		}
		if strings.HasPrefix(blah, "GOBIN") {
			env[i] = pathbin
			found = true
		}
	}
	if !found {
		env = append(env, path)
	}
	c.command.Env = env

	out, _ := c.command.StdoutPipe()
	if out != nil {
		scanner := bufio.NewScanner(out)
		go func() {
			for scanner != nil && scanner.Scan() {
				c.output += scanner.Text()
			}
			out.Close()
		}()
	}

	out2, _ := c.command.StderrPipe()
	if out2 != nil {
		scanner := bufio.NewScanner(out2)
		go func() {
			for scanner != nil && scanner.Scan() {
				c.erroutput += scanner.Text()
			}
			out2.Close()
		}()
	}

	c.command.Dir = "/home/simon/gobuild/bin"
	err := c.command.Start()
	if err != nil {
		return err
	}
	c.startTime = time.Now().Unix()

	// Monitor the job and report completion
	go func() {
		err := c.command.Wait()
		c.endTime = time.Now().Unix()
		if err != nil {
			c.err = err
		}
	}()

	return nil
}
