// AGENTV1 FILE START: validation-only CLI, never a fake telemetry daemon.
package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/agent-i/agent/internal/app"
	"github.com/agent-i/agent/internal/config"
	"github.com/agent-i/agent/internal/platform"
	"github.com/agent-i/agent/internal/version"
	"io"
	"os"
)

func run(args []string, out, errOut io.Writer) int {
	flags := flag.NewFlagSet("observe-agent", flag.ContinueOnError)
	flags.SetOutput(errOut)
	file := flags.String("config", platform.Defaults().Config, "foundation JSON configuration")
	check := flags.Bool("check", false, "validate without secrets, identity reads, or collector startup")
	// AGENTV1 START: explicit metrics runtime, never implicit installation or deployment
	start := flags.Bool("run", false, "run enabled Linux metrics collector until stopped")
	// AGENTV1 END: explicit metrics runtime
	show := flags.Bool("version", false, "show build version")
	if flags.Parse(args) != nil {
		return 2
	}
	if *show {
		fmt.Fprintln(out, version.Version)
		return 0
	}
	if !*check && !*start {
		fmt.Fprintln(errOut, "use --check to validate or --run for the Linux metrics-only runtime")
		return 2
	}
	// AGENTV1 START: protected YAML load and offline auth validation.
	cfg, err := config.Load(*file)
	if err != nil {
		fmt.Fprintln(errOut, err) // Config errors are explicitly redacted by the loader/parser.
		return 1
	}
	if err = cfg.CheckCredentials(context.Background()); err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}
	// AGENTV1 END: protected configuration preflight
	if *start && !*check {
		if err = app.Run(context.Background(), cfg, errOut); err != nil {
			fmt.Fprintln(errOut, err)
			return 1
		}
		return 0
	}
	fmt.Fprintln(out, "Configuration valid. No collectors, listeners or network requests started; YAML credentials validated offline.")
	return 0
}
func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

// AGENTV1 FILE END
