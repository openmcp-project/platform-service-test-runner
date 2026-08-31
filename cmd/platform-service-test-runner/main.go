package main

import (
	"context"
	"fmt"
	"os"

	"github.com/openmcp-project/platform-service-test-runner/cmd/platform-service-test-runner/app"

	"github.com/openmcp-project/controller-utils/pkg/fips"
)

func main() {
	fips.Verify(context.Background())

	cmd := app.NewPlatformServiceTestRunnerCommand()

	if err := cmd.Execute(); err != nil {
		fmt.Print(err)
		os.Exit(1)
	}
}
