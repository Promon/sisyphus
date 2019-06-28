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
	"strconv"
	"syscall"
	"time"
)

const (
	BurstLimit                   = 5
	DefaultActiveDeadlineSeconds = 3600
)

func init() {
	// Initialize logger
	log.SetLevel(log.DebugLevel)
	log.SetFormatter(&log.TextFormatter{
		ForceColors:            true,
		FullTimestamp:          true,
		DisableLevelTruncation: true,
	})

	log.SetOutput(os.Stdout)
}

func readConf(inClusterOpt *bool) (*conf.SisyphusConf, error) {
	var confPath string
	flag.StringVar(&confPath, "conf", "", "The `conf.yaml` file")
	flag.BoolVar(inClusterOpt, "in-cluster", false, "Use in-cluster config, when running inside cluster")
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
	var inCluster = false
	sConf, err := readConf(&inCluster)
	if err != nil {
		log.Panic(err)
	}
	log.Infof("In Cluster config: %v", inCluster)

	// Parse request quantities
	var defaultRequests v1.ResourceList
	if len(sConf.DefaultResourceRequest) > 0 {
		defaultRequests, err = conf.ParseResourceQuantity(sConf.DefaultResourceRequest)
		if err != nil {
			log.Panic(err)
		}
	}

	if err = conf.ValidateDefaultResourceQuantity(defaultRequests); err != nil {
		log.Panic(err)
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

			// Parse custom job parameters passed via env variables
			vars := protocol.GetEnvVars(j)
			//noinspection GoShadowedVar
			resReq, err := loadCustomK8SJobParams(vars, defaultRequests, sConf.DefaultNodeSelector)
			if err != nil {
				log.Error(err)
				continue
			}

			//noinspection GoShadowedVar
			k8sSession, err := kubernetes.CreateK8SSession(inCluster, sConf.K8SNamespace)
			if err != nil {
				log.Error(err)
			}
			go jobmon.RunJob(j, k8sSession, resReq, httpSession, sConf.GcpCacheBucket, workOk)

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

//
func loadCustomK8SJobParams(envVars map[string]string,
	defaultResourceRequest v1.ResourceList,
	defaultNodeSelector map[string]string) (*kubernetes.K8SJobParameters, error) {

	var params = kubernetes.K8SJobParameters{}

	// Custom resource requests merged with default ones
	reqVal, ok := envVars[shell.SfsResourceRequest]
	if ok {
		req, err := parseCustomResourceRequests(reqVal)
		if err != nil {
			return nil, err
		}

		// Override with defaults
		for k, dv := range defaultResourceRequest {
			if _, ok = req[k]; !ok {
				req[k] = dv
			}
		}

		params.ResourceRequest = req
	} else {
		params.ResourceRequest = defaultResourceRequest
	}

	// Custom deadline
	dVal, ok := envVars[shell.SfsActiveDeadline]
	if ok {
		dLine, err := strconv.ParseInt(dVal, 10, 64)
		if err != nil {
			return nil, err
		}

		params.ActiveDeadlineSec = dLine
	} else {
		params.ActiveDeadlineSec = DefaultActiveDeadlineSeconds
	}

	// Custom node selector
	nSel, ok := envVars[shell.SfsNodeSelector]
	if ok {
		customNsel := make(map[string]string)
		err := json.Unmarshal([]byte(nSel), &customNsel)
		if err != nil {
			return nil, err
		}

		params.NodeSelector = customNsel
	} else {
		params.NodeSelector = defaultNodeSelector
	}

	return &params, nil
}

func parseCustomResourceRequests(envVal string) (v1.ResourceList, error) {
	resourceLst := make(v1.ResourceList)
	err := json.Unmarshal([]byte(envVal), &resourceLst)
	if err != nil {
		return nil, err
	}

	return resourceLst, err
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
