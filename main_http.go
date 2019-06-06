package main

import "sisyphus/protocol"

func main() {
	session, err := protocol.NewHttpSession("https://git.dev.promon.no", "AaNZWjA47W9FTin2gGGh")
	if err != nil {
		panic(err)
	}

	err = session.PollProjects()
	if err != nil {
		panic(err)
	}
}
