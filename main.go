package main

import (
	"encoding/json"
	"errors"
	"flag"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	v1 "k8s.io/api/core/v1"
	"os"
	"os/signal"
	"sisyphus/conf"
	"sisyphus/jobmon"
	"sisyphus/kubernetes"
	"sisyphus/protocol"
	"sisyphus/shell"
	"syscall"
	"time"
)

const (
	BurstLimit = 5
)

func init() {
	// Initialize logger
	log.SetLevel(log.DebugLevel)
	log.SetFormatter(&log.TextFormatter{
		ForceColors:   true,
		FullTimestamp: true,
	})
	log.SetOutput(os.Stdout)
}

func readConf() (*conf.SisyphusConf, error) {
	var confPath string
	flag.StringVar(&confPath, "conf", "", "The `conf.yaml` file")
	flag.Parse()

	if len(confPath) == 0 {
		return nil, errors.New("no configuration file provided. Use --conf")
	}

	rawConf, err := ioutil.ReadFile(confPath)
	if err != nil {
		return nil, err
	}

	sConf, err := conf.ReadSisyphusConf(rawConf)
	if err != nil {
		return nil, err
	}

	return sConf, nil
}

func main() {
	log.Info("Hello.")
	sConf, err := readConf()
	if err != nil {
		log.Panic(err)
	}

	// Parse request quantities
	var defaultRequests v1.ResourceList
	if len(sConf.DefaultResourceRequest) > 0 {
		defaultRequests, err = conf.ParseResourceQuantity(sConf.DefaultResourceRequest)
		if err != nil {
			log.Panic(err)
		}
	}

	httpSession, err := protocol.NewHttpSession(sConf.GitlabUrl)
	if err != nil {
		log.Panic(err)
	}

	// Queue with ticks
	workOk := make(chan bool, BurstLimit)

	// Queue for new jobs from gitlab
	newJobs := make(chan *protocol.JobSpec, BurstLimit)
	go nextJobLoop(httpSession, sConf.RunnerToken, newJobs, workOk)

	// Handle OS signals
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	// Ticker used to rate limit requests
	ticktock := time.NewTicker(100 * time.Millisecond)
	defer ticktock.Stop()

	// Main event loop
	for {
		select {
		case <-ticktock.C:
			// Allocate 1 job monitoring cycle per tick
			if len(workOk) < BurstLimit {
				workOk <- true
			}

		case j := <-newJobs:
			ji := j.JobInfo
			log.Infof("New job received. proj=%s stage=%s name=%s", ji.ProjectName, ji.Stage, ji.Name)

			k8sSession, erra := kubernetes.CreateK8SSession(sConf.K8SNamespace)
			if erra != nil {
				log.Error(erra)
			} else {
				vars := protocol.GetEnvVars(j)
				resReq, errb := getCustomResourceRequests(vars)

				if err != nil {
					log.Error(errb)
				} else {
					var resRequest = defaultRequests
					if resReq != nil {
						resRequest = *resReq
					}

					go jobmon.RunJob(j, k8sSession, resRequest, httpSession, sConf.GcpCacheBucket, workOk)
				}
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

func getCustomResourceRequests(envVars map[string]string) (*v1.ResourceList, error) {
	jsv, ok := envVars[shell.SfsResourceRequest]
	if ok {
		var resourceQuant []conf.ResourceQuantity
		err := json.Unmarshal([]byte(jsv), &resourceQuant)
		if err != nil {
			return nil, err
		}
		result, err := conf.ParseResourceQuantity(resourceQuant)
		return &result, err
	} else {
		return nil, nil
	}
}

func nextJobLoop(httpSession *protocol.RunnerHttpSession, runnerToken string, newJobs chan<- *protocol.JobSpec, workOk <-chan bool) {
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
