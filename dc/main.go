package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// StdoutResponseWriter implements http.ResponseWriter and http.Flusher
// and writes all output to stdout (headers/status to stderr).
type StdoutResponseWriter struct {
	h http.Header
}

func NewStdoutResponseWriter() *StdoutResponseWriter {
	return &StdoutResponseWriter{h: make(http.Header)}
}

func (w *StdoutResponseWriter) Header() http.Header { return w.h }
func (w *StdoutResponseWriter) WriteHeader(statusCode int) {

}
func (w *StdoutResponseWriter) Write(b []byte) (int, error) { return os.Stdout.Write(b) }
func (w *StdoutResponseWriter) Flush()                      { _ = os.Stdout.Sync() }

func (w *StdoutResponseWriter) WriteHeaderString(statusLine string) {

}

func main() {
	// Buffer all log output so that successful invocations (e.g. "dc stacks ls")
	// produce clean stdout with no diagnostic noise. Logs are only flushed to
	// stderr on failure via die().
	var logBuf bytes.Buffer
	log.SetOutput(&logBuf)

	die := func(format string, args ...interface{}) {
		if logBuf.Len() > 0 {
			os.Stderr.Write(logBuf.Bytes())
		}
		fmt.Fprintf(os.Stderr, format+"\n", args...)
		os.Exit(1)
	}

	// Initialize paths first (respects --stacks-dir and --env-path arguments)
	InitPaths(os.Args)

	// Keep compatibility with flags that might be passed; ignore unknowns
	host := flag.String("host", "", "(ignored) Server host")
	flag.Parse()
	_ = host

	args := flag.Args()
	if len(args) < 1 {
		die("Usage: dc stack <command> [name]\nCommands: ls, start|up, stop, down, logs")
	}

	writer := NewStdoutResponseWriter()

	switch args[0] {
	case "stack", "stacks":
		if len(args) < 2 {
			die("Usage: dc stack <command> [name]")
		}
		cmd := args[1]
		switch cmd {
		case "ls", "list":
			r := &http.Request{Method: http.MethodGet, URL: &url.URL{Path: "/api/stacks"}}
			HandleListStacks(writer, r)
		case "start", "up":
			if len(args) < 3 {
				die("Usage: dc stack %s <name>", cmd)
			}
			name := args[2]
			yamlBody, err := findYAML(name)
			if err != nil {
				die("%v", err)
			}
			r := &http.Request{
				Method: http.MethodPut,
				URL:    &url.URL{Path: "/api/stacks/" + name + "/start"},
				Body:   io.NopCloser(bytes.NewReader(yamlBody)),
			}
			HandleStartStack(writer, r)
		case "stop":
			if len(args) < 3 {
				die("Usage: dc stack stop <name>")
			}
			name := args[2]
			r := &http.Request{Method: http.MethodPut, URL: &url.URL{Path: "/api/stacks/" + name + "/stop"}}
			HandleStopStack(writer, r)
		case "down", "rm", "remove":
			if len(args) < 3 {
				die("Usage: dc stack %s <name>", cmd)
			}
			name := args[2]
			r := &http.Request{Method: http.MethodDelete, URL: &url.URL{Path: "/api/stacks/" + name}}
			HandleDeleteStack(writer, r)
		case "logs":
			if len(args) < 3 {
				die("Usage: dc stack logs <name>")
			}
			name := args[2]
			r := &http.Request{Method: http.MethodGet, URL: &url.URL{Path: "/api/stacks/" + name + "/logs"}}
			HandleStreamStackLogs(writer, r)
		default:
			die("Unknown stack command: %s", cmd)
		}

	case "pw", "secret":
		// Forward pw/secret commands to an external `pw` script which reads/writes the env store.
		if len(args) < 2 {
			die("Usage: dc %s <args...>", args[0])
		}
		cmdArgs := args[1:]
		// Normalize common long verbs to short aliases (insert/delete/update/upsert/get -> ins/del/upd/ups/get)
		if len(cmdArgs) > 0 {
			switch strings.ToLower(cmdArgs[0]) {
			case "generate":
				cmdArgs[0] = "gen"
			case "insert", "add":
				cmdArgs[0] = "ins"
			case "delete", "remove", "rm":
				cmdArgs[0] = "del"
			case "update":
				cmdArgs[0] = "upd"
			case "upsert":
				cmdArgs[0] = "ups"
			case "select":
				cmdArgs[0] = "get"
			}
		}
		// Determine helper executable via configuration key `secrets_manager` (falls back to "pw").
		script := getConfig("secrets_manager", "pw")
		if script == "" {
			script = "pw"
		}
		// If script is a simple name, prefer PATH; otherwise if it contains a path use that directly when present.
		if !strings.ContainsAny(script, string(os.PathSeparator)) {
			if _, err := exec.LookPath(script); err != nil {
				// fallback to relative ./dc/<script> or next to executable
				candidate := filepath.Join(".", "dc", script)
				if _, err2 := os.Stat(candidate); err2 == nil {
					script = candidate
				} else if ex, err3 := os.Executable(); err3 == nil {
					alt := filepath.Join(filepath.Dir(ex), script)
					if _, err4 := os.Stat(alt); err4 == nil {
						script = alt
					}
				}
			}
		} else {
			// script contains a path; prefer it if it exists, otherwise attempt basename in PATH or fallbacks
			if fi, err := os.Stat(script); err == nil && fi.Mode().IsRegular() {
				// use provided path
			} else {
				base := filepath.Base(script)
				if _, err := exec.LookPath(base); err == nil {
					script = base
				} else {
					candidate := filepath.Join(".", "dc", base)
					if _, err2 := os.Stat(candidate); err2 == nil {
						script = candidate
					} else if ex, err3 := os.Executable(); err3 == nil {
						alt := filepath.Join(filepath.Dir(ex), base)
						if _, err4 := os.Stat(alt); err4 == nil {
							script = alt
						}
					}
				}
			}
		}
		cmd := exec.Command(script, cmdArgs...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			die("pw command failed: %v", err)
		}

	default:
		die("Unknown command: %s", args[0])
	}
}

// findYAML searches common locations for a stack YAML file. Kept from the previous CLI logic.
func findYAML(name string) ([]byte, error) {
	home, _ := os.UserHomeDir()
	u := os.Getenv("USER")

	candidates := []string{
		fmt.Sprintf("./%s.yml", name),
		filepath.Join(home, "/stacks", name+".yml"),
		filepath.Join(home, ".local/stacks", name+".yml"),
		filepath.Join(home, ".dotfiles/users", u, ".local/stacks", name+".yml"),
		filepath.Join(home, "/containers", name+".yml"),
		filepath.Join(home, ".local/containers", name+".yml"),
		filepath.Join(home, ".dotfiles/users", u, ".local/containers", name+".yml"),
	}

	for _, p := range candidates {
		data, err := os.ReadFile(p)
		if err == nil {
			return data, nil
		}
	}
	return nil, fmt.Errorf("no YAML found for stack %q; tried: %v", name, candidates)
}

// The following old functions (login, run) are intentionally left as no-ops
// to preserve compatibility with other callers in the repo; they are not used
// by the new CLI entrypoint but kept to avoid breaking the build.
func login(host, user, password string) (string, error)        { return "", nil }
func run(host, token, command, name string, body []byte) error { return nil }
