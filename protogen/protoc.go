package protogen

import (
	"bytes"
	"context"
	"io"
	"strings"

	protoc "github.com/aperturerobotics/go-protoc-wasi"
	"github.com/pkg/errors"
	"github.com/tetratelabs/wazero"
)

// ProtocRun is one protoc invocation inside the embedded WASI runtime.
type ProtocRun struct {
	// Plugins resolves the plugin programs that protoc requests.
	Plugins *Plugins
	// Mounts are host directories that protoc reads or writes, visible at
	// the same path inside the runtime.
	Mounts []string
	// Args are the protoc arguments after the program name.
	Args []string
	// Verbose prints the command line and protoc output to Stdout.
	Verbose bool
	// Stdout receives verbose output. It may be nil when Verbose is false.
	Stdout io.Writer
}

// Run executes protoc and returns an error when it exits unsuccessfully.
func (r *ProtocRun) Run(ctx context.Context) error {
	// Create the wazero runtime and the handler that executes plugins.
	runtime := wazero.NewRuntime(ctx)
	defer runtime.Close(ctx)
	pluginHandler := NewNativePluginHandler(r.Plugins, r.Verbose)

	// Mount the directories protoc reads schemas from and writes output to.
	fsConfig := wazero.NewFSConfig()
	for _, mount := range r.Mounts {
		fsConfig = fsConfig.WithDirMount(mount, mount)
	}

	// Create and initialize protoc, which instantiates WASI.
	var stdout, stderr bytes.Buffer
	p, err := protoc.NewProtoc(ctx, runtime, &protoc.Config{
		Stdout:        &stdout,
		Stderr:        &stderr,
		FSConfig:      fsConfig,
		PluginHandler: pluginHandler,
	})
	if err != nil {
		return errors.Wrap(err, "failed to create protoc")
	}
	defer p.Close(ctx)
	if err := p.Init(ctx); err != nil {
		return errors.Wrap(err, "failed to init protoc")
	}

	// The prost WASM plugin needs the WASI instance that protoc created.
	if r.Plugins.RustProst != nil {
		if err := pluginHandler.InitProstWASM(ctx, runtime); err != nil {
			return errors.Wrap(err, "failed to init prost WASM")
		}
		defer pluginHandler.CloseProstWASM(ctx)
	}

	// Run protoc and report its exit status.
	args := append([]string{"protoc"}, r.Args...)
	if r.Verbose {
		if _, err := io.WriteString(r.Stdout, "Running: "+strings.Join(args, " ")+"\n"); err != nil {
			return err
		}
	}

	// Preserve protoc's failure and diagnostic text for the command caller.
	exitCode, err := p.Run(ctx, args)
	if err != nil {
		return errors.Wrap(err, "protoc error")
	}
	if exitCode != 0 {
		if stderr.Len() > 0 {
			return errors.Errorf("protoc failed with exit code %d: %s", exitCode, stderr.String())
		}
		return errors.Errorf("protoc failed with exit code %d", exitCode)
	}

	// Successful protoc output is visible only in verbose mode.
	if r.Verbose && stdout.Len() > 0 {
		if _, err := io.WriteString(r.Stdout, stdout.String()); err != nil {
			return err
		}
	}
	return nil
}
