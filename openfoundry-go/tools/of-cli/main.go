// Command of is the Go port of the Rust tools/of-cli utility.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
)

const usage = `of — OpenFoundry CLI

Commands:
  docs generate-openapi --proto-dir <dir> --output <file>
  docs validate-openapi --proto-dir <dir> --expected <file>
  docs generate-sdk-typescript --input <openapi.json> --output <dir>
  docs validate-sdk-typescript --input <openapi.json> --output <dir>
  docs generate-sdk-python --input <openapi.json> --output <dir>
  docs validate-sdk-python --input <openapi.json> --output <dir>
  docs generate-sdk-java --input <openapi.json> --output <dir>
  docs validate-sdk-java --input <openapi.json> --output <dir>
  smoke run --scenario <file> --output <file>
  bench run --scenario <file> --output <file>
  mock-provider serve [--host 127.0.0.1] [--port 18080]
`

type commandKind string

const (
	cmdGenerateOpenAPI   commandKind = "docs generate-openapi"
	cmdValidateOpenAPI   commandKind = "docs validate-openapi"
	cmdGenerateSDKTS     commandKind = "docs generate-sdk-typescript"
	cmdValidateSDKTS     commandKind = "docs validate-sdk-typescript"
	cmdGenerateSDKPython commandKind = "docs generate-sdk-python"
	cmdValidateSDKPython commandKind = "docs validate-sdk-python"
	cmdGenerateSDKJava   commandKind = "docs generate-sdk-java"
	cmdValidateSDKJava   commandKind = "docs validate-sdk-java"
	cmdSmokeRun          commandKind = "smoke run"
	cmdBenchRun          commandKind = "bench run"
	cmdBenchmarkRun      commandKind = "benchmark run"
	cmdMockProviderServe commandKind = "mock-provider serve"
)

type cliConfig struct {
	kind     commandKind
	protoDir string
	input    string
	expected string
	output   string
	scenario string
	host     string
	port     int
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := runCLI(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runCLI(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	cfg, err := parseArgs(args, stderr)
	if err != nil {
		return err
	}
	switch cfg.kind {
	case cmdGenerateOpenAPI:
		return generateOpenAPI(cfg.protoDir, cfg.output)
	case cmdValidateOpenAPI:
		return validateOpenAPI(cfg.protoDir, cfg.expected)
	case cmdGenerateSDKTS:
		return generateSDK(cfg.input, cfg.output, "typescript")
	case cmdValidateSDKTS:
		return validateSDK(cfg.input, cfg.output, "typescript")
	case cmdGenerateSDKPython:
		return generateSDK(cfg.input, cfg.output, "python")
	case cmdValidateSDKPython:
		return validateSDK(cfg.input, cfg.output, "python")
	case cmdGenerateSDKJava:
		return generateSDK(cfg.input, cfg.output, "java")
	case cmdValidateSDKJava:
		return validateSDK(cfg.input, cfg.output, "java")
	case cmdSmokeRun:
		return runSmokeSuite(ctx, cfg.scenario, cfg.output)
	case cmdBenchRun, cmdBenchmarkRun:
		return runBenchmarkSuite(ctx, cfg.scenario, cfg.output)
	case cmdMockProviderServe:
		return serveMockProvider(ctx, cfg.host, cfg.port, stdout)
	default:
		return errors.New(usage)
	}
}

func parseArgs(args []string, stderr io.Writer) (cliConfig, error) {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		return cliConfig{}, errors.New(usage)
	}

	subcommand := ""
	if len(args) > 1 {
		subcommand = args[1]
	}

	switch args[0] {
	case "docs":
		if subcommand == "" {
			return cliConfig{}, errors.New("missing docs subcommand\n\n" + usage)
		}
		return parseDocsArgs(subcommand, args[2:], stderr)
	case "smoke":
		if subcommand != "run" {
			return cliConfig{}, fmt.Errorf("unknown smoke subcommand %q\n\n%s", subcommand, usage)
		}
		return parseScenarioArgs(cmdSmokeRun, args[2:], stderr)
	case "bench":
		if subcommand != "run" {
			return cliConfig{}, fmt.Errorf("unknown bench subcommand %q\n\n%s", subcommand, usage)
		}
		return parseScenarioArgs(cmdBenchRun, args[2:], stderr)
	case "benchmark":
		if subcommand != "run" {
			return cliConfig{}, fmt.Errorf("unknown benchmark subcommand %q\n\n%s", subcommand, usage)
		}
		return parseScenarioArgs(cmdBenchmarkRun, args[2:], stderr)
	case "mock-provider":
		if subcommand != "serve" {
			return cliConfig{}, fmt.Errorf("unknown mock-provider subcommand %q\n\n%s", subcommand, usage)
		}
		fs := newFlagSet("mock-provider serve", stderr)
		cfg := cliConfig{kind: cmdMockProviderServe, host: "127.0.0.1", port: 18080}
		fs.StringVar(&cfg.host, "host", cfg.host, "host/interface to bind")
		fs.IntVar(&cfg.port, "port", cfg.port, "TCP port to bind")
		if err := fs.Parse(args[2:]); err != nil {
			return cliConfig{}, err
		}
		if cfg.port <= 0 || cfg.port > 65535 {
			return cliConfig{}, fmt.Errorf("invalid --port %d", cfg.port)
		}
		return cfg, nil
	default:
		return cliConfig{}, fmt.Errorf("unknown command %q\n\n%s", args[0], usage)
	}
}

func parseDocsArgs(subcommand string, args []string, stderr io.Writer) (cliConfig, error) {
	fs := newFlagSet("docs "+subcommand, stderr)
	cfg := cliConfig{kind: commandKind("docs " + subcommand)}
	switch cfg.kind {
	case cmdGenerateOpenAPI:
		cfg.protoDir = "../proto"
		fs.StringVar(&cfg.protoDir, "proto-dir", cfg.protoDir, "directory containing .proto files")
		fs.StringVar(&cfg.output, "output", "", "OpenAPI JSON output path")
	case cmdValidateOpenAPI:
		cfg.protoDir = "../proto"
		fs.StringVar(&cfg.protoDir, "proto-dir", cfg.protoDir, "directory containing .proto files")
		fs.StringVar(&cfg.expected, "expected", "", "expected OpenAPI JSON path")
	case cmdGenerateSDKTS, cmdValidateSDKTS, cmdGenerateSDKPython, cmdValidateSDKPython, cmdGenerateSDKJava, cmdValidateSDKJava:
		fs.StringVar(&cfg.input, "input", "", "OpenAPI JSON input path")
		fs.StringVar(&cfg.output, "output", "", "SDK output directory")
	default:
		return cliConfig{}, fmt.Errorf("unknown docs subcommand %q\n\n%s", subcommand, usage)
	}
	if err := fs.Parse(args); err != nil {
		return cliConfig{}, err
	}
	if (cfg.kind == cmdGenerateOpenAPI && cfg.output == "") || (cfg.kind == cmdValidateOpenAPI && cfg.expected == "") {
		return cliConfig{}, fmt.Errorf("%s requires --output/--expected", cfg.kind)
	}
	if (cfg.kind != cmdGenerateOpenAPI && cfg.kind != cmdValidateOpenAPI) && (cfg.input == "" || cfg.output == "") {
		return cliConfig{}, fmt.Errorf("%s requires --input and --output", cfg.kind)
	}
	return cfg, nil
}

func parseScenarioArgs(kind commandKind, args []string, stderr io.Writer) (cliConfig, error) {
	fs := newFlagSet(string(kind), stderr)
	cfg := cliConfig{kind: kind}
	fs.StringVar(&cfg.scenario, "scenario", "", "scenario JSON path")
	fs.StringVar(&cfg.output, "output", "", "report JSON output path")
	if err := fs.Parse(args); err != nil {
		return cliConfig{}, err
	}
	if cfg.scenario == "" || cfg.output == "" {
		return cliConfig{}, fmt.Errorf("%s requires --scenario and --output", kind)
	}
	return cfg, nil
}

func newFlagSet(name string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	return fs
}
