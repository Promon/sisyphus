package shell

import (
	"fmt"
	"sisyphus/protocol"
	"testing"
)

func TestGenerateScript(t *testing.T) {
	script := GenerateScript(&protocol.JobSpec{}, "TEST")
	fmt.Println(script)
}
