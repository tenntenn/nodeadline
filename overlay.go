package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/build"
	"go/format"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

//go:embed _partials/nodeadline.go
var nodeadline string

func goVersion(modroot string) (string, error) {
	var stdout bytes.Buffer
	cmd := exec.Command("go", "env", "GOVERSION")
	cmd.Dir = modroot
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return "", err
	}

	return strings.TrimSpace(stdout.String()), nil
}

func goRoot(modroot string) (string, error) {
	var stdout bytes.Buffer
	cmd := exec.Command("go", "env", "GOROOT")
	cmd.Dir = modroot
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return "", err
	}

	return strings.TrimSpace(stdout.String()), nil
}

func createOverlay(update bool, modroot, output string) (string, error) {

	ver, err := goVersion(modroot)
	if err != nil {
		return "", err
	}

	overlay := filepath.Join(output, fmt.Sprintf("overlay_%s.json", ver))
	_, err = os.Stat(overlay)
	switch {
	case err == nil:
		if !update {
			return overlay, nil
		}
	case !errors.Is(err, os.ErrNotExist):
		return "", err
	}

	if err := os.MkdirAll(output, 0o700); err != nil {
		return "", err
	}

	goroot, err := goRoot(modroot)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	old, err := replaceWithDeadlineCause(&buf, goroot)
	if err != nil {
		return "", err
	}

	fmt.Fprint(&buf, nodeadline)

	src, err := format.Source(buf.Bytes())
	if err != nil {
		return "", fmt.Errorf("format error: %w", err)
	}

	new := filepath.Join(output, fmt.Sprintf("context_%s.go", ver))
	if err := os.WriteFile(new, src, 0o600); err != nil {
		return "", err
	}

	v := struct {
		Replace map[string]string
	}{map[string]string{old: new}}
	jsonBytes, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(overlay, jsonBytes, 0o600); err != nil {
		return "", err
	}

	return overlay, nil
}

func replaceWithDeadlineCause(w io.Writer, goroot string) (string, error) {
	srcDir := filepath.Join(goroot, "src")
	ctx := build.Default
	ctx.GOROOT = goroot
	pkg, err := ctx.Import("context", srcDir, 0)
	if err != nil {
		return "", fmt.Errorf("import error: %w", err)
	}

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, pkg.Dir, nil, parser.ParseComments)
	if err != nil {
		return "", fmt.Errorf("parse error: %w", err)
	}

	if pkgs["context"] == nil {
		return "", errors.New("cannot find context package")
	}

	var (
		path   string
		syntax *ast.File
	)
LOOP:
	for name, file := range pkgs["context"].Files {
		for _, decl := range file.Decls {
			decl, _ := decl.(*ast.FuncDecl)
			if decl == nil {
				continue
			}

			if decl.Name.Name == "WithDeadlineCause" {
				decl.Name.Name = "_WithDeadlineCause"
				path = name
				syntax = file
				break LOOP
			}
		}
	}

	if path == "" || syntax == nil {
		return "", errors.New("cannot find context.WithDeadlineCause")
	}

	if err := format.Node(w, fset, syntax); err != nil {
		return "", fmt.Errorf("format error: %w", err)
	}

	return path, nil
}
