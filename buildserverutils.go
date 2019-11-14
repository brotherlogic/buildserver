package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	pb "github.com/brotherlogic/buildserver/proto"
	"github.com/golang/protobuf/proto"
)

func (s *Server) preloadInfo() error {
	return filepath.Walk(s.dir, func(path string, info os.FileInfo, err error) error {
		if strings.HasSuffix(path, ".version") {
			data, _ := ioutil.ReadFile(path)
			val := &pb.Version{}
			if len(data) > 0 {
				proto.Unmarshal(data, val)
				s.pathMapMutex.Lock()
				s.pathMap[path[:len(path)-len(".version")]] = val

				jobn := val.Job.Name

				found := false
				for _, job := range s.jobs {
					if job.Name == jobn {
						found = true
					}
				}
				if !found {
					s.jobs = append(s.jobs, val.Job)
				}

				if val.VersionDate > s.latestBuild[jobn] {
					s.latestBuild[jobn] = val.VersionDate
					s.latestHash[jobn] = val.GithubHash
					s.latestDate[jobn] = time.Unix(val.VersionDate, 0)
					s.latestVersion[jobn] = val.Version
				}
				s.pathMapMutex.Unlock()
			}
		}
		return nil
	})
}
