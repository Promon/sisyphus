package main

import (
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
	BurstLimit  = 5
	CacheBucket = "gitlab_ci_cache"
)

func main() {
	log.Info("Hello.")

	k8sSession, err := kubernetes.CreateK8SSession("default")
	if err != nil {
		log.Panic(err)
	}

	httpSession, err := protocol.NewHttpSession("https://git.dev.promon.no")
	if err != nil {
		log.Panic(err)
	}

	// Queue with ticks
	workOk := make(chan bool, BurstLimit)

	// Queue for new jobs from gitlab
	newJobs := make(chan *protocol.JobSpec, BurstLimit)
	go nextJobLoop(httpSession, newJobs, workOk)

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

		case j := <-newJobs:
			go jobmon.RunJob(j, k8sSession, httpSession, CacheBucket, workOk)

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

func nextJobLoop(httpSession *protocol.RunnerHttpSession, newJobs chan<- *protocol.JobSpec, workOk <-chan bool) {
	for range workOk {
		nextJob, err := httpSession.PollNextJob(runnerToken)
		if err != nil {
			log.Warn(err)
		} else if nextJob != nil {
			newJobs <- nextJob
		}
	}

	log.Info("Work fetch loop terminated")
}
