//go:build ignore

// build.go is a cross-platform build helper invoked by the Makefile.
// It replaces shell-dependent recipes (mkdir -p, rm -rf, inline env vars)
// so that "make build", "make release", and "make clean" work on Windows
// (GnuWin32 make / cmd.exe) as well as Linux and macOS.
//
// Usage (via Makefile):
//
//	go run ./scripts/build.go build
//	go run ./scripts/build.go install
//	go run ./scripts/build.go lint
//	go run ./scripts/build.go release
//	go run ./scripts/build.go clean
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	dist = "dist"
	pkg  = "./cmd/bolt-cowork"
)

var releaseTargets = []struct {
	goos, goarch, suffix string
}{
	{"windows", "amd64", ".exe"},
	{"linux", "amd64", ""},
	{"linux", "arm64", ""},
	{"darwin", "amd64", ""},
	{"darwin", "arm64", ""},
}

func main() {
	if len(os.Args) < 2 {
		fatalf("usage: build.go <build|install|lint|release|clean>")
	}

	target := os.Args[1]
	version := detectVersion()

	switch target {
	case "build":
		mustMkdir(dist)
		suffix := ""
		if runtime.GOOS == "windows" {
			suffix = ".exe"
		}
		goBuild("", "", version, filepath.Join(dist, "bolt-cowork"+suffix))

	case "install":
		goInstall(version)

	case "lint":
		checkGofmt()
		runCommand("go", "vet", "./...")
		runCommand("golangci-lint", "run", "./...")

	case "release":
		mustRemoveAll(dist)
		mustMkdir(dist)
		for _, t := range releaseTargets {
			name := fmt.Sprintf("bolt-cowork-%s-%s%s", t.goos, t.goarch, t.suffix)
			fmt.Printf("-> %s\n", name)
			goBuild(t.goos, t.goarch, version, filepath.Join(dist, name))
		}

	case "clean":
		mustRemoveAll(dist)
		// Remove legacy root-level binaries from before dist/ was introduced.
		_ = os.Remove("bolt-cowork")
		_ = os.Remove("bolt-cowork.exe")

	default:
		fatalf("unknown target: %s", target)
	}
}

func checkGofmt() {
	files := goPackageFiles()
	if len(files) == 0 {
		return
	}
	args := append([]string{"-l"}, files...)
	cmd := exec.Command("gofmt", args...)
	out, err := cmd.Output()
	if err != nil {
		fatalf("gofmt -l: %v", err)
	}
	formatted := strings.TrimSpace(string(out))
	if formatted == "" {
		return
	}
	fmt.Fprintln(os.Stderr, "Run gofmt on these files:")
	fmt.Fprintln(os.Stderr, formatted)
	os.Exit(1)
}

func goPackageFiles() []string {
	var files []string
	for _, root := range []string{"cmd", "internal", "pkg", "scripts"} {
		if _, err := os.Stat(root); os.IsNotExist(err) {
			continue
		}
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			if strings.HasSuffix(entry.Name(), ".go") {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			fatalf("walk %s: %v", root, err)
		}
	}
	return files
}

func detectVersion() string {
	cmd := exec.Command("git", "describe", "--tags", "--always", "--dirty")
	out, err := cmd.Output()
	if err != nil {
		return "dev"
	}
	version := strings.TrimSpace(string(out))
	if version == "" {
		return "dev"
	}
	return version
}

func goBuild(goos, goarch, version, output string) {
	ldflags := fmt.Sprintf("-X main.version=%s", version)
	args := []string{"build", "-ldflags", ldflags, "-o", output, pkg}

	cmd := exec.Command("go", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	env := os.Environ()
	if goos != "" {
		env = setenv(env, "GOOS", goos)
		env = setenv(env, "GOARCH", goarch)
		env = setenv(env, "CGO_ENABLED", "0")
	}
	cmd.Env = env

	if err := cmd.Run(); err != nil {
		fatalf("go build %s: %v", output, err)
	}
}

func runCommand(name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fatalf("%s %s: %v", name, strings.Join(args, " "), err)
	}
}

func goInstall(version string) {
	ldflags := fmt.Sprintf("-X main.version=%s", version)
	cmd := exec.Command("go", "install", "-ldflags", ldflags, pkg)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fatalf("go install: %v", err)
	}
}

// setenv sets key=val in a copy of the env slice, replacing any existing entry.
func setenv(env []string, key, val string) []string {
	prefix := key + "="
	for i, e := range env {
		if strings.HasPrefix(e, prefix) {
			env[i] = prefix + val
			return env
		}
	}
	return append(env, prefix+val)
}

func mustMkdir(dir string) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fatalf("mkdir %s: %v", dir, err)
	}
}

func mustRemoveAll(dir string) {
	if err := os.RemoveAll(dir); err != nil {
		fatalf("remove %s: %v", dir, err)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "build: "+format+"\n", args...)
	os.Exit(1)
}
