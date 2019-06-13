package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	v1 "k8s.io/api/core/v1"
	"os"
	"sisyphus/kubernetes"
	"sisyphus/protocol"
	"time"
)

func init() {
	log.SetLevel(log.DebugLevel)
	log.SetFormatter(&log.TextFormatter{
		ForceColors: true,
	})
	log.SetOutput(os.Stdout)
}

func main() {
	log.Info("Hello.")

	s, err := kubernetes.CreateK8SSession("default")
	if err != nil {
		log.Panic(err)
	}

	gitlabJob, err := loadSampleGitLabJob("protocol/testdata/job_spec.json")
	if err != nil {
		log.Panic(err)
	}

	job, err := s.CreateGitLabJob("testjob", gitlabJob)
	if err != nil {
		log.Panic(err)
	}
	defer job.Delete()

	//
	//job, err := s.CreateJob("testjob")
	//if err != nil {
	//	panic(err.Error())
	//}
	//defer job.Delete()
	//
	for i := 0; i < 100; i++ {
		status, err := job.GetReadinessStatus()
		if err != nil {
			fmt.Println(err.Error())
		} else {
			s := toJson(status)
			fmt.Println(s)

			if status.PodPhaseCounts[v1.PodUnknown] == 0 && status.PodPhaseCounts[v1.PodPending] == 0 {
				break
			}
		}

		time.Sleep(1 * time.Second)
	}
	//
	rdr, err := job.GetLog()
	if err != nil {
		log.Panic(err)
	}
	defer rdr.Close()

	sc := bufio.NewScanner(rdr)
	for sc.Scan() {
		fmt.Println(sc.Text())
	}

	if err := sc.Err(); err != nil {
		log.Panic(err.Error())
	}
}

func toJson(i interface{}) string {
	b, err := json.MarshalIndent(i, "", " ")
	if err != nil {
		panic(err.Error())
	}

	return string(b)
}

func loadSampleGitLabJob(path string) (*protocol.JobSpec, error) {
	log.Debugf("Loading %v", path)
	jsonData, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	result, err := protocol.ParseJobSpec(jsonData)
	if err != nil {
		return nil, err
	}

	log.Debugf("Successfully loaded %v", path)
	return result, nil
}
