package main

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"os"
	"os/signal"
	"sisyphus/jobmon"
	"sisyphus/kubernetes"
	"sisyphus/protocol"
	"syscall"
	"time"
)

func init() {
	log.SetLevel(log.DebugLevel)
	log.SetFormatter(&log.TextFormatter{
		ForceColors:   true,
		FullTimestamp: true,
	})
	log.SetOutput(os.Stdout)
}

const runnerToken = "kxZppSfxQjM6aAmAoxjo"
const (
	// Limit for monitoring requests
	BurstLimit = 5
)

func main() {
	log.Info("Hello.")

	s, err := kubernetes.CreateK8SSession("default")
	if err != nil {
		log.Panic(err)
	}

	//gitlabJob, err := loadSampleGitLabJob("protocol/testdata/job_spec.json")
	//if err != nil {
	//	log.Panic(err)
	//}

	httpSession, err := protocol.NewHttpSession("https://git.dev.promon.no")
	if err != nil {
		log.Panic(err)
	}

	nextJob, err := httpSession.PollNextJob(runnerToken)
	if err != nil {
		log.Panic(err)
	}

	if nextJob == nil {
		// no new jobs to run
		log.Info("No jobs to run")
		os.Exit(0)
	}

	jobPreifx := fmt.Sprintf("sphs-%v-%v-", nextJob.JobInfo.ProjectId, nextJob.Id)
	job, err := s.CreateGitLabJob(jobPreifx, nextJob)
	if err != nil {
		log.Panic(err)
	}
	//defer job.Delete()

	workOk := make(chan bool, BurstLimit)

	go jobmon.MonitorJob(job, httpSession, nextJob.Id, nextJob.Token, workOk)

	// Handle OS signals
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	// Ticker used to rate limit requests
	ticktock := time.NewTicker(500 * time.Millisecond)

	// Main event loop
	for {
		select {
		case <-ticktock.C:
			// Allocate 1 job monitoring cycle per tick
			if len(workOk) < BurstLimit {
				workOk <- true
			}

		case s := <-signals:
			log.Debugf("Signal received %v", s)
			close(workOk)
			time.Sleep(5 * time.Second)
			return

		default:
			log.Trace("No activity")
			time.Sleep(1 * time.Second)
		}
	}
}
