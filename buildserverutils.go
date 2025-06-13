package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/protobuf/proto"

	pb "github.com/brotherlogic/buildserver/proto"
)

func (s *Server) preloadInfo(ctx context.Context) error {
	return filepath.Walk(s.dir, func(path string, info os.FileInfo, err error) error {
		if strings.HasSuffix(path, ".version") {
			s.CtxLog(ctx, fmt.Sprintf("Reading %v", path))
			data, _ := ioutil.ReadFile(path)
			val := &pb.Version{}
			if len(data) > 0 {
				err := proto.Unmarshal(data, val)
				if err != nil {
					s.CtxLog(ctx, fmt.Sprintf("Unable to read: %v (%v)", path, err))
				} else {
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
						s.latest[jobn] = val
					}
				}
			}
		}
		return nil
	})
}
