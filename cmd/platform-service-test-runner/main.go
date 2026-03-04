package main

import (
	"fmt"
	"os"

	"github.com/openmcp-project/platform-service-test-runner/cmd/platform-service-test-runner/app"
)

func main() {
	cmd := app.NewPlatformServiceTestRunnerCommand()

	if err := cmd.Execute(); err != nil {
		fmt.Print(err)
		os.Exit(1)
	}
}
