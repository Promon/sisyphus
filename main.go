package main

import (
	"bufio"
	"ciproxy/kubernetes"
	"encoding/json"
	"fmt"
	v1 "k8s.io/api/core/v1"
	"time"
)

func main() {
	s, err := kubernetes.CreateK8SSession("default")
	if err != nil {
		panic(err.Error())
	}

	job, err := s.CreateJob("testjob")
	if err != nil {
		panic(err.Error())
	}
	defer job.Delete()

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

	rdr, err := job.GetLog()
	if err != nil {
		panic(err.Error())
	}
	defer rdr.Close()

	sc := bufio.NewScanner(rdr)
	for sc.Scan() {
		fmt.Println(sc.Text())
	}

	if err := sc.Err(); err != nil {
		panic(err.Error())
	}
}

func toJson(i interface{}) string {
	b, err := json.MarshalIndent(i, "", " ")
	if err != nil {
		panic(err.Error())
	}

	return string(b)
}
