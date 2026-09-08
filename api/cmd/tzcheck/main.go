// Command tzcheck is a build-only probe, never one of the six vecingest
// subcommands and never part of the production image's default build
// target. It exists solely to prove, from inside the exact same
// gcr.io/distroless/static-debian12:nonroot base image, that
// time.LoadLocation("Europe/Madrid") succeeds — a regression test for the
// `time/tzdata` embed in cmd/vecingest/main.go (design.md cross-cutting
// rule 3; task 8.4/8.5). See api/Dockerfile's "tzcheck" build stage and
// api/test/docker_tzcheck_test.go, which builds and runs this exact stage.
package main

import (
	"fmt"
	"os"
	"time"
	_ "time/tzdata"
)

func main() {
	loc, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		fmt.Fprintf(os.Stderr, "tzcheck: LoadLocation(Europe/Madrid) failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("tzcheck: ok zone=%s\n", loc.String())
}
