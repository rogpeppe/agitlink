package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"9fans.net/go/acme"
)

func main() {
	if err := main1(); err != nil {
		fmt.Fprintf(os.Stderr, "agitlink: %v\n", err)
		os.Exit(1)
	}
}

func main1() error {
	f, err := acmeReadCurrentFile()
	if err != nil {
		return err
	}
	gitRepo, err := repo(filepath.Dir(f.name))
	if err != nil {
		return fmt.Errorf("cannot get repo: %v", err)
	}
	relPath, err := relativeFilename(f.name)
	if err != nil {
		return fmt.Errorf("cannot get relative path: %v", err)
	}
	gitCommit, err := commit(f.name)
	if err != nil {
		return fmt.Errorf("cannot get commit: %v", err)
	}
	l0, l1 := lineNumber(f.body, f.runeOffset0, true), lineNumber(f.body, f.runeOffset1, false)
	url := fmt.Sprintf("https://github.com/%s/blob/%s/%s", gitRepo, gitCommit, relPath)
	if l1 > l0 {
		url += fmt.Sprintf("#L%d-L%d", l0, l1)
	} else {
		url += fmt.Sprintf("#L%d", l0)
	}
	fmt.Printf("%s\n", url)
	return nil
}

const remotePrefix = "git@github.com:"

func repo(dir string) (string, error) {
	var buf bytes.Buffer
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Stderr = os.Stderr
	cmd.Stdout = &buf
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		return "", err
	}
	s := buf.String()
	s1 := strings.TrimPrefix(s, remotePrefix)
	if len(s1) == len(s) {
		return "", fmt.Errorf("unexpected prefix for remote %q (want %q)", s, remotePrefix)
	}
	return strings.TrimSpace(s1), nil
}

func relativeFilename(f string) (string, error) {
	var buf bytes.Buffer
	cmd := exec.Command("git", "rev-parse", "--show-prefix")
	cmd.Stderr = os.Stderr
	cmd.Dir = filepath.Dir(f)
	cmd.Stdout = &buf
	if err := cmd.Run(); err != nil {
		return "", err
	}
	relDir := strings.TrimSpace(buf.String())
	return path.Join(relDir, filepath.Base(f)), nil
}

func commit(f string) (string, error) {
	var buf bytes.Buffer
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Stderr = os.Stderr
	cmd.Dir = filepath.Dir(f)
	cmd.Stdout = &buf
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(buf.String()), nil
}

type acmeFile struct {
	name        string
	body        []byte
	runeOffset0 int
	runeOffset1 int
}

func acmeReadCurrentFile() (*acmeFile, error) {
	win, err := acmeCurrentWin()
	if err != nil {
		return nil, err
	}
	defer win.CloseFiles()
	_, _, err = win.ReadAddr() // make sure address file is already open.
	if err != nil {
		return nil, fmt.Errorf("cannot read address: %v", err)
	}
	err = win.Ctl("addr=dot")
	if err != nil {
		return nil, fmt.Errorf("cannot set addr=dot: %v", err)
	}
	q0, q1, err := win.ReadAddr()
	if err != nil {
		return nil, fmt.Errorf("cannot read address: %v", err)
	}
	body, err := readBody(win)
	if err != nil {
		return nil, fmt.Errorf("cannot read body: %v", err)
	}
	tagb, err := win.ReadAll("tag")
	if err != nil {
		return nil, fmt.Errorf("cannot read tag: %v", err)
	}
	tag := string(tagb)
	i := strings.Index(tag, " ")
	if i == -1 {
		return nil, fmt.Errorf("strange tag with no spaces")
	}
	return &acmeFile{
		name:        tag[0:i],
		body:        body,
		runeOffset0: q0,
		runeOffset1: q1,
	}, nil
}

// We would use win.ReadAll except for a bug in acme
// where it crashes when reading trying to read more
// than the negotiated 9P message size.
func readBody(win *acme.Win) ([]byte, error) {
	var body []byte
	buf := make([]byte, 8000)
	for {
		n, err := win.Read("body", buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		body = append(body, buf[0:n]...)
	}
	return body, nil
}

func acmeCurrentWin() (*acme.Win, error) {
	winid := os.Getenv("winid")
	if winid == "" {
		return nil, fmt.Errorf("$winid not set - not running inside acme?")
	}
	id, err := strconv.Atoi(winid)
	if err != nil {
		return nil, fmt.Errorf("invalid $winid %q", winid)
	}
	win, err := acme.Open(id, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot open acme window: %v", err)
	}
	return win, nil
}

func lineNumber(data []byte, runeOffset int, start bool) int {
	line := 1
	off := 0
	for len(data) > 0 {
		if start && off >= runeOffset {
			return line
		}
		r, n := utf8.DecodeRune(data)
		data = data[n:]
		off++
		if !start && off >= runeOffset {
			return line
		}
		if r == '\n' {
			line++
		}
	}
	if line > 1 {
		// There's no line after the final newline, so use the
		// previous line instead.
		line--
	}
	return line
}
