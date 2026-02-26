package runner

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openmcp-project/controller-utils/pkg/logging"
)

var ctx context.Context

func TestRunner(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Runner Suite")
}

var _ = BeforeSuite(func() {
	ctx = logging.NewContext(context.Background(), logging.Discard())
})
