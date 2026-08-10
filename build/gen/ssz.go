package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type sszTarget struct {
	pkg, out string
	libInc   []string
	protoInc []string
	objs     []string
	exclude  []string
}

func genSSZ() error {
	targets, err := loadSSZTargets()
	if err != nil {
		return fmt.Errorf("load SSZ targets: %w", err)
	}

	ps, err := loadPresets()
	if err != nil {
		return fmt.Errorf("load presets: %w", err)
	}

	// The base preset's .pb.go tree is the checked-in one (root ""); every other
	// preset is regenerated into its own temp tree.
	nonBase := ps.nonBase()

	roots := make(map[string]string, len(nonBase))
	if len(nonBase) > 0 {
		g, err := newProtoGen()
		if err != nil {
			return fmt.Errorf("new proto gen: %w", err)
		}
		defer g.cleanup()

		for _, preset := range nonBase {
			root := filepath.Join(g.root, "pbgo", preset)
			if err := g.emit(ps.dicts[preset], root); err != nil {
				return fmt.Errorf("emit %s pb.go: %w", preset, err)
			}

			roots[preset] = root
		}
	}

	for _, target := range targets {
		if err := genSSZTarget(target, ps, roots); err != nil {
			return fmt.Errorf("gen SSZ target: %w", err)
		}
	}

	return nil
}

func genSSZTarget(t sszTarget, ps presetSet, roots map[string]string) error {
	fmt.Printf("generating %s/%s\n", t.pkg, t.out)

	nonBase := ps.nonBase()

	base, err := sszgenOne(t, "", nonBase)
	if err != nil {
		return fmt.Errorf("%s: %w", ps.base(), err)
	}

	// Presets whose output is byte-identical to the base one need no twin.
	twins := make(map[string]string, len(nonBase))
	var differing []string
	for _, preset := range nonBase {
		out, err := sszgenOne(t, roots[preset], nonBase)
		if err != nil {
			return fmt.Errorf("%s: %w", preset, err)
		}

		if out != base {
			differing = append(differing, preset)
			twins[preset] = out
		}
	}

	content := base
	if tag := buildTag(differing); tag != "" {
		content = "//go:build " + tag + "\n\n" + base
	}

	if err := os.WriteFile(filepath.Join(t.pkg, t.out), []byte(content), 0o600); err != nil {
		return fmt.Errorf("writeFile: %w", err)
	}

	for _, preset := range nonBase {
		path := filepath.Join(t.pkg, presetTwin(t.out, ".ssz.go", preset))
		out, ok := twins[preset]
		if !ok {
			// Drop a stale twin from an earlier run: the base file is not
			// negated for this preset, so the twin would shadow it.
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove %s: %w", path, err)
			}

			continue
		}

		if err := os.WriteFile(path, []byte("//go:build "+preset+"\n\n"+out), 0o600); err != nil {
			return fmt.Errorf("writeFile: %w", err)
		}
	}

	return nil
}

func sszgenOne(t sszTarget, root string, presets []string) (string, error) {
	stage := filepath.Join(root, t.pkg, ".sszgen_tmp")
	if err := stagePbgo(filepath.Join(root, t.pkg), stage, presets); err != nil {
		return "", fmt.Errorf("stagePbgo: %w", err)
	}

	defer unstage(stage)

	inc := slices.Clone(t.libInc)
	for _, p := range t.protoInc {
		istage := filepath.Join(root, p, ".sszinc_tmp")
		if err := stagePbgo(filepath.Join(root, p), istage, presets); err != nil {
			return "", fmt.Errorf("stagePbgo: %w", err)
		}

		defer unstage(istage)
		inc = append(inc, istage)
	}

	tmp, err := os.CreateTemp("", "sszgen-*.go")
	if err != nil {
		return "", fmt.Errorf("createTemp: %w", err)
	}

	tmpName := tmp.Name()
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("close: %w", err)
	}

	defer func() { _ = os.Remove(tmpName) }()

	args := []string{"--output=" + tmpName, "--path=" + stage, "--objs=" + strings.Join(t.objs, ",")}
	if len(inc) > 0 {
		args = append(args, "--include="+strings.Join(inc, ","))
	}

	if len(t.exclude) > 0 {
		args = append(args, "--exclude-objs="+strings.Join(t.exclude, ","))
	}

	if err := sh("go", append([]string{"tool", "sszgen"}, args...)...); err != nil {
		return "", fmt.Errorf("sh: %w", err)
	}

	data, err := os.ReadFile(tmpName) // #nosec G304 -- tmpName is our own os.CreateTemp output
	if err != nil {
		return "", fmt.Errorf("readFile: %w", err)
	}

	var b strings.Builder
	for _, line := range strings.SplitAfter(string(data), "\n") {
		if strings.Contains(line, "// Hash: ") {
			continue
		}

		b.WriteString(line)
	}

	return b.String(), nil
}

// stagePbgo copies pkgDir's .pb.go files into stageDir, skipping the twins of
// the given presets.
//
// The twins are not fed to sszgen: each non-base preset is regenerated from the
// .proto files into a temp dir at gen time, and they are proto outputs already
// covered by the proto manifest.
func stagePbgo(pkgDir, stageDir string, presets []string) error {
	if err := os.MkdirAll(stageDir, 0o750); err != nil {
		return fmt.Errorf("mkdirAll: %w", err)
	}

	entries, err := os.ReadDir(pkgDir)
	if err != nil {
		return fmt.Errorf("readDir: %w", err)
	}

	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".pb.go") || isPresetTwin(name, ".pb.go", presets) {
			continue
		}

		data, err := os.ReadFile(filepath.Join(pkgDir, name)) // #nosec G304 -- pkgDir/name from a controlled ReadDir of repo proto packages
		if err != nil {
			return fmt.Errorf("readFile: %w", err)
		}

		if err := os.WriteFile(filepath.Join(stageDir, name), data, 0o600); err != nil {
			return fmt.Errorf("writeFile: %w", err)
		}
	}

	return nil
}

func unstage(dir string) { _ = os.RemoveAll(dir) }
