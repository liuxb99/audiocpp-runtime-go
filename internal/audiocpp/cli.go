package audiocpp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"time"
)

type CLIExecutor struct {
	cliPath    string
	workingDir string
	timeout    time.Duration
}

func NewCLIExecutor(cliPath, workingDir string, timeout time.Duration) *CLIExecutor {
	return &CLIExecutor{
		cliPath:    cliPath,
		workingDir: workingDir,
		timeout:    timeout,
	}
}

type CLIArgs struct {
	Task          string
	Family        string
	ModelPath     string
	Backend       string
	Device        int
	Threads       int
	Text          string
	Audio         string
	VoiceRef      string
	ReferenceText string
	Seed          int
	Language      string
	Out           string
	Extra         map[string]string
}

func (e *CLIExecutor) TTS(ctx context.Context, args *CLIArgs) ([]byte, error) {
	cmdArgs := e.buildArgs(args)
	ctx, cancel := e.timeoutContext(ctx)
	defer cancel()

	cmd := exec.CommandContext(ctx, e.cliPath, cmdArgs...)
	if e.workingDir != "" {
		cmd.Dir = e.workingDir
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, e.mapExecError(err, output)
	}

	return output, nil
}

func (e *CLIExecutor) Transcribe(ctx context.Context, args *CLIArgs) (string, error) {
	cmdArgs := e.buildArgs(args)
	ctx, cancel := e.timeoutContext(ctx)
	defer cancel()

	cmd := exec.CommandContext(ctx, e.cliPath, cmdArgs...)
	if e.workingDir != "" {
		cmd.Dir = e.workingDir
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", e.mapExecError(err, output)
	}

	return string(output), nil
}

func (e *CLIExecutor) Run(ctx context.Context, args *CLIArgs) (map[string]interface{}, error) {
	cmdArgs := e.buildArgs(args)
	ctx, cancel := e.timeoutContext(ctx)
	defer cancel()

	cmd := exec.CommandContext(ctx, e.cliPath, cmdArgs...)
	if e.workingDir != "" {
		cmd.Dir = e.workingDir
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, e.mapExecError(err, output)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, NewError(ErrInternal, "failed to parse CLI JSON output", err.Error())
	}
	return result, nil
}

func (e *CLIExecutor) buildArgs(args *CLIArgs) []string {
	var cmdArgs []string

	cmdArgs = append(cmdArgs, "--task", args.Task)

	if args.Family != "" {
		cmdArgs = append(cmdArgs, "--family", args.Family)
	}
	if args.ModelPath != "" {
		cmdArgs = append(cmdArgs, "--model", args.ModelPath)
	}
	if args.Backend != "" {
		cmdArgs = append(cmdArgs, "--backend", args.Backend)
	}
	if args.Device != 0 {
		cmdArgs = append(cmdArgs, "--device", strconv.Itoa(args.Device))
	}
	if args.Threads != 0 {
		cmdArgs = append(cmdArgs, "--threads", strconv.Itoa(args.Threads))
	}
	if args.Text != "" {
		cmdArgs = append(cmdArgs, "--text", args.Text)
	}
	if args.Audio != "" {
		cmdArgs = append(cmdArgs, "--audio", args.Audio)
	}
	if args.VoiceRef != "" {
		cmdArgs = append(cmdArgs, "--voice-ref", args.VoiceRef)
	}
	if args.ReferenceText != "" {
		cmdArgs = append(cmdArgs, "--reference-text", args.ReferenceText)
	}
	if args.Seed != 0 {
		cmdArgs = append(cmdArgs, "--seed", strconv.Itoa(args.Seed))
	}
	if args.Language != "" {
		cmdArgs = append(cmdArgs, "--language", args.Language)
	}
	if args.Out != "" {
		cmdArgs = append(cmdArgs, "--out", args.Out)
	}

	for k, v := range args.Extra {
		cmdArgs = append(cmdArgs, "--"+k, v)
	}

	return cmdArgs
}

func (e *CLIExecutor) timeoutContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if e.timeout > 0 {
		return context.WithTimeout(ctx, e.timeout)
	}
	return context.WithCancel(ctx)
}

func (e *CLIExecutor) mapExecError(err error, output []byte) *Error {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return NewError(ErrRequestTimeout, "cli process timed out", err.Error())
	}
	msg := string(output)
	if msg == "" {
		msg = err.Error()
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return NewError(ErrProcessCrash, fmt.Sprintf("cli exited with code %d", exitErr.ExitCode()), msg)
	}
	return NewError(ErrProcessCrash, "cli execution failed", msg)
}
