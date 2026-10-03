package protogen

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/aperturerobotics/protobuf-go-lite/types/descriptorpb"
	"github.com/pkg/errors"
)

// rustFileModules is the protoc-gen-prost option that keys each generated
// module by its source schema instead of its protobuf package, so that every
// schema gets its own file even when a package spans several directories.
const rustFileModules = "file_modules"

// rustMessageSuffix ends the file name of generated Rust messages.
const rustMessageSuffix = ".pb.rs"

// rustServiceSuffix ends the stem of generated Rust service bindings.
const rustServiceSuffix = "_srpc"

// RustGenerator generates Rust protobuf bindings for the whole schema graph.
//
// Unlike Generator, which caches per directory, it compiles every schema in
// one protoc run so that type resolution sees the full graph, and it owns the
// complete output set: messages, service bindings, module declarations, the
// reflection descriptor set, and the inventory used to remove obsolete files.
// Output is built in memory and compared with the project before any write,
// which is what makes a read-only drift check possible.
type RustGenerator struct {
	// Config is the generator configuration.
	Config *Config
	// Rust is the Rust section of the project configuration.
	Rust *RustConfig
	// Plugins contains the Rust plugins.
	Plugins *Plugins
	// ProjectDir is the resolved project directory and crate root.
	ProjectDir string
	// ModulePath is the import path prefix of the project's schemas.
	ModulePath string
	// Stdout is where to write verbose output.
	Stdout io.Writer
}

// NewRustGenerator creates a RustGenerator from the aptre.rust configuration.
func NewRustGenerator(cfg *Config) (*RustGenerator, error) {
	// Resolve the project identity and Rust configuration.
	projectDir, err := cfg.GetProjectDir()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get project directory")
	}

	// Validate the whole-graph selection and keep its outputs inside the project.
	rust, err := cfg.GetRust()
	if err != nil {
		return nil, errors.Wrap(err, "failed to read aptre.rust")
	}
	if rust == nil {
		return nil, errors.New("aptre.rust is not configured in package.json")
	}
	for _, output := range []string{rust.DescriptorSet, rust.ModuleFile, rust.Inventory} {
		if output != "" && !filepath.IsLocal(filepath.FromSlash(output)) {
			return nil, errors.Errorf("aptre.rust output %q must stay inside the project", output)
		}
	}

	// A schema module can be configured without a Go module.
	modulePath, err := cfg.GetSchemaModule()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get schema module")
	}

	// Select the plugins; a missing service plugin would silently drop every
	// service file and delete the existing ones as obsolete.
	toolsDir, err := cfg.GetToolsDir()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get tools directory")
	}

	// Select the service libraries before deciding which plugin must be present.
	rpcs, err := cfg.GetRPCLibraries()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get RPC libraries")
	}

	// Require selected services instead of silently deleting their previous output.
	starpc, prost := discoverRustPlugins(filepath.Join(toolsDir, "bin"), rpcs)
	if rpcs.Has(RPCLibraryStarpc) && starpc == nil {
		return nil, errors.Errorf("protoc-gen-starpc-rust is not built in %s; run aptre deps", filepath.Join(toolsDir, "bin"))
	}
	prost.Flags = append([]string{rustFileModules}, rust.ProstOptions...)

	// Services name the same message types, so they get the options that decide
	// how a type is named.
	if starpc != nil {
		for _, option := range rust.ProstOptions {
			if isRustTypeOption(option) {
				starpc.Flags = append(starpc.Flags, option)
			}
		}
	}

	return &RustGenerator{
		Config: cfg,
		Rust:   rust,
		Plugins: &Plugins{
			Languages:    Languages{LanguageRust: {}},
			RPCLibraries: rpcs,
			RustStarpc:   starpc,
			RustProst:    prost,
		},
		ProjectDir: projectDir,
		ModulePath: modulePath,
		Stdout:     os.Stdout,
	}, nil
}

// isRustTypeOption reports whether a Prost option decides how a message type is
// named, which the service plugin must apply to resolve the same paths.
func isRustTypeOption(option string) bool {
	name, _, _ := strings.Cut(option, "=")
	return name == "extern_path" || name == "compile_well_known_types"
}

// Generate writes the changed outputs and removes obsolete ones. It returns
// the project-relative paths it wrote or removed.
func (g *RustGenerator) Generate(ctx context.Context) ([]string, error) {
	outputs, err := g.render(ctx)
	if err != nil {
		return nil, err
	}
	return g.publish(outputs, false)
}

// Check compares the project with freshly generated output without writing.
// It returns the project-relative paths that are outdated, missing, or obsolete.
func (g *RustGenerator) Check(ctx context.Context) ([]string, error) {
	outputs, err := g.render(ctx)
	if err != nil {
		return nil, err
	}
	return g.publish(outputs, true)
}

// render builds every output in memory, keyed by project-relative slash path.
func (g *RustGenerator) render(ctx context.Context) (map[string][]byte, error) {
	// Select the schemas that compile as roots; their imports must be tracked
	// too, so every schema in the graph gets an output.
	protoFiles, err := DiscoverProtoFiles(g.ProjectDir, g.Config.Targets, slices.Concat(g.Config.Exclude, g.Rust.Exclude))
	if err != nil {
		return nil, errors.Wrap(err, "failed to discover proto files")
	}
	if len(protoFiles) == 0 {
		return nil, errors.New("no proto files found")
	}
	slices.Sort(protoFiles)

	// Generate into a scratch directory so the project stays untouched.
	scratch, err := os.MkdirTemp("", "aptre-rust-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(scratch)

	// Compile the full graph and its descriptor set in the same invocation.
	descriptorPath := filepath.Join(scratch, ".descriptor-set")
	if err := g.runProtoc(ctx, protoFiles, scratch, descriptorPath); err != nil {
		return nil, errors.Wrap(err, "failed to generate protos")
	}

	// Move each generated file to the directory of its schema.
	outputs := make(map[string][]byte)
	err = filepath.WalkDir(scratch, func(file string, entry fs.DirEntry, err error) error {
		// Only generated regular outputs enter the publication set.
		if err != nil || entry.IsDir() || file == descriptorPath {
			return err
		}

		// Map the plugin's canonical import path back to the schema's project location.
		rel, err := filepath.Rel(scratch, file)
		if err != nil {
			return err
		}
		dest := g.projectPath(filepath.ToSlash(rel))
		if !strings.HasSuffix(dest, rustMessageSuffix) {
			return errors.Errorf("unexpected generated file %s", dest)
		}

		// Retain complete bytes so comparison finishes before publication begins.
		data, err := os.ReadFile(file)
		if err != nil {
			return err
		}

		// A schema with only services or extern types declares nothing to include.
		if declaresRustItems(data) {
			outputs[dest] = data
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Read the descriptor set of the whole graph; the module declarations and
	// runtime reflection both use it.
	descriptors, err := os.ReadFile(descriptorPath)
	if err != nil {
		return nil, err
	}

	// Declare each output under the module of its protobuf package.
	if g.Rust.ModuleFile != "" {
		packages, err := g.schemaPackages(descriptors)
		if err != nil {
			return nil, err
		}
		declarations, err := g.renderModules(packages, slices.Sorted(maps.Keys(outputs)))
		if err != nil {
			return nil, err
		}
		outputs[g.Rust.ModuleFile] = []byte("// @generated by aptre.\n" + declarations)
	}

	// Keep the descriptor set for runtime reflection.
	if g.Rust.DescriptorSet != "" {
		outputs[g.Rust.DescriptorSet] = descriptors
	}

	// List the outputs so a later run can remove those of deleted schemas.
	var inventory strings.Builder
	for _, file := range slices.Sorted(maps.Keys(outputs)) {
		inventory.WriteString(file + "\n")
	}
	outputs[g.Rust.Inventory] = []byte(inventory.String())

	return outputs, nil
}

// declaresRustItems reports whether generated Rust has a line that is neither
// blank nor a comment. Prost writes only its header for a schema whose messages
// all map to extern paths or that declares only services.
func declaresRustItems(data []byte) bool {
	for line := range strings.Lines(string(data)) {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "//") {
			return true
		}
	}
	return false
}

// runProtoc compiles all schemas in one run, writing Rust output to outDir and
// the descriptor set of the whole graph to descriptorPath.
func (g *RustGenerator) runProtoc(ctx context.Context, protoFiles []string, outDir, descriptorPath string) error {
	// Canonical imports of this project resolve to the project directory and
	// all others to the vendor directory, so no schema is copied or linked.
	vendorDir := filepath.Join(g.ProjectDir, "vendor")
	args := []string{
		"--proto_path=" + g.ModulePath + "=" + g.ProjectDir,
		"--proto_path=" + vendorDir,
	}

	// Add the include path of the google well-known proto types.
	protobufSrcDir := filepath.Join(vendorDir, "github.com", "aperturerobotics", "protobuf", "src")
	if _, err := os.Stat(protobufSrcDir); err == nil {
		args = append(args, "--proto_path="+protobufSrcDir)
	}

	// Select the plugin outputs, the descriptor set, and the schemas.
	args = append(args, g.Plugins.GetProtocArgs(outDir, outDir)...)
	args = append(args, "--descriptor_set_out="+descriptorPath, "--include_imports", "--include_source_info")
	args = append(args, g.Config.ExtraArgs...)
	for _, file := range protoFiles {
		args = append(args, g.importPath(file))
	}

	// The shared runner owns WASI, mounts and plugin process cleanup.
	run := &ProtocRun{
		Plugins: g.Plugins,
		Mounts:  []string{g.ProjectDir, outDir},
		Args:    args,
		Verbose: g.Config.Verbose,
		Stdout:  g.Stdout,
	}
	return run.Run(ctx)
}

// renderModules declares the generated files under their protobuf packages.
// packages maps the project-relative path of each schema without its extension
// to its protobuf package, as recorded in the compiled descriptor set.
func (g *RustGenerator) renderModules(packages map[string]string, generated []string) (string, error) {
	// A service file belongs to the package of the schema that declares it.
	modules := NewRustModules()
	for _, file := range generated {
		stem := strings.TrimSuffix(file, rustMessageSuffix)
		protoPackage, ok := packages[stem]
		if !ok {
			stem = strings.TrimSuffix(stem, rustServiceSuffix)
			protoPackage, ok = packages[stem]
		}
		if !ok {
			return "", errors.Errorf("generated file %s has no source schema", file)
		}
		modules.Add(protoPackage, file)
	}
	return modules.Render(), nil
}

// schemaPackages reads the protobuf package of every compiled schema from the
// descriptor set, keyed by the project-relative schema path without its
// extension. The package comes from the compiled schema, never from its text.
func (g *RustGenerator) schemaPackages(descriptors []byte) (map[string]string, error) {
	// Decode the compiler's authoritative package metadata.
	set := &descriptorpb.FileDescriptorSet{}
	if err := set.UnmarshalVT(descriptors); err != nil {
		return nil, errors.Wrap(err, "read descriptor set")
	}

	// Index package identities by the paths used for generated output.
	packages := make(map[string]string, len(set.GetFile()))
	for _, file := range set.GetFile() {
		name := strings.TrimSuffix(g.projectPath(file.GetName()), ".proto")
		packages[name] = file.GetPackage()
	}
	return packages, nil
}

// importPath returns the canonical import path of a project-relative schema.
// Schemas under vendor are dependencies, which keep their original import path.
func (g *RustGenerator) importPath(file string) string {
	if dependency, ok := strings.CutPrefix(file, "vendor/"); ok {
		return dependency
	}
	return path.Join(g.ModulePath, file)
}

// projectPath is the inverse of importPath for a generated file's path.
func (g *RustGenerator) projectPath(importPath string) string {
	if projectFile, ok := strings.CutPrefix(importPath, g.ModulePath+"/"); ok {
		return projectFile
	}
	return path.Join("vendor", importPath)
}

// publish compares outputs with the project and, unless check is set, writes
// the changed files, removes obsolete ones, and writes the inventory last so a
// failed run can still remove the outputs the previous inventory names.
// It returns the project-relative paths that changed or became obsolete.
func (g *RustGenerator) publish(outputs map[string][]byte, check bool) ([]string, error) {
	// Compare every output before writing so a check reports all drift.
	var changed []string
	for _, file := range slices.Sorted(maps.Keys(outputs)) {
		current, err := os.ReadFile(g.absPath(file))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		if err != nil || !bytes.Equal(current, outputs[file]) {
			changed = append(changed, file)
		}
	}

	// An output named by the previous inventory but no longer generated is
	// obsolete. The inventory is not trusted to name only files in the project.
	previous, err := os.ReadFile(g.absPath(g.Rust.Inventory))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	var obsolete []string
	for line := range strings.Lines(string(previous)) {
		file := strings.TrimSpace(line)
		if _, ok := outputs[file]; ok || file == "" {
			continue
		}
		if !filepath.IsLocal(filepath.FromSlash(file)) {
			return nil, errors.Errorf("inventory %s names %q outside the project", g.Rust.Inventory, file)
		}
		obsolete = append(obsolete, file)
	}

	// Read-only checks return the same complete drift set without publishing it.
	stale := slices.Concat(changed, obsolete)
	if check || len(stale) == 0 {
		return stale, nil
	}

	// Write changed files, then remove obsolete ones.
	for _, file := range changed {
		if file == g.Rust.Inventory {
			continue
		}
		if err := g.writeFile(file, outputs[file]); err != nil {
			return nil, err
		}
	}
	for _, file := range obsolete {
		if err := os.Remove(g.absPath(file)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}

	// Publish the inventory last.
	if slices.Contains(changed, g.Rust.Inventory) {
		if err := g.writeFile(g.Rust.Inventory, outputs[g.Rust.Inventory]); err != nil {
			return nil, err
		}
	}
	return stale, nil
}

// writeFile writes a project-relative file, creating its directory.
func (g *RustGenerator) writeFile(file string, data []byte) error {
	target := g.absPath(file)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, data, 0o644) //nolint:gosec
}

// absPath resolves a project-relative slash path.
func (g *RustGenerator) absPath(file string) string {
	return filepath.Join(g.ProjectDir, filepath.FromSlash(file))
}
