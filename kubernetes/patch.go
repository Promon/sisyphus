package kubernetes

import (
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
)

func genPatch(original interface{}, modified interface{}) ([]byte, error) {
	origBytes, err := json.Marshal(original)
	if err != nil {
		return nil, err
	}

	modBytes, err := json.Marshal(modified)
	if err != nil {
		return nil, err
	}

	return strategicpatch.CreateTwoWayMergePatch(origBytes, modBytes, modified)
}
