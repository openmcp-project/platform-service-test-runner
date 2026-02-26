package util

import (
	"github.com/openmcp-project/platform-service-test-runner/api/v1alpha1"
)

func GetStatus(name string, testCaseStati []v1alpha1.TestCaseStatus) (*v1alpha1.TestCaseStatus, bool) {
	for i, tC := range testCaseStati {
		if tC.Name == name {
			return &testCaseStati[i], true
		}
	}
	return nil, false
}
