package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
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
	fmt.Fprintf(os.Stderr, "Status: %d\n", statusCode)
}
func (w *StdoutResponseWriter) Write(b []byte) (int, error) { return os.Stdout.Write(b) }
func (w *StdoutResponseWriter) Flush()                      { _ = os.Stdout.Sync() }
func (w *StdoutResponseWriter) WriteString(s string) (int, error) {
	return io.WriteString(os.Stdout, s)
}
func (w *StdoutResponseWriter) WriteHeaderString(statusLine string) {
	fmt.Fprintln(os.Stderr, statusLine)
}

func main() {
	// Initialize paths first (respects --stacks-dir and --env-path arguments)
	InitPaths(os.Args)

	// Keep compatibility with flags that might be passed; ignore unknowns
	host := flag.String("host", "", "(ignored) Server host")
	user := flag.String("user", "", "(ignored) Username")
	password := flag.String("password", "", "(ignored) Password")
	flag.Parse()
	_ = host
	_ = user
	_ = password

	args := flag.Args()
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: dc stack <command> [name]\nCommands: ls, start|up, stop, down, logs\n")
		os.Exit(1)
	}

	writer := NewStdoutResponseWriter()

	switch args[0] {
	case "stack", "stacks":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Usage: dc stack <command> [name]\n")
			os.Exit(1)
		}
		cmd := args[1]
		switch cmd {
		case "ls", "list":
			r := &http.Request{Method: http.MethodGet, URL: &url.URL{Path: "/api/stacks"}}
			HandleListStacks(writer, r)
		case "start", "up":
			if len(args) < 3 {
				fmt.Fprintf(os.Stderr, "Usage: dc stack %s <name>\n", cmd)
				os.Exit(1)
			}
			name := args[2]
			yamlBody, err := findYAML(name)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(1)
			}
			r := &http.Request{
				Method: http.MethodPut,
				URL:    &url.URL{Path: "/api/stacks/" + name + "/start"},
				Body:   io.NopCloser(bytes.NewReader(yamlBody)),
			}
			HandleStartStack(writer, r)
		case "stop":
			if len(args) < 3 {
				fmt.Fprintf(os.Stderr, "Usage: dc stack stop <name>\n")
				os.Exit(1)
			}
			name := args[2]
			r := &http.Request{Method: http.MethodPut, URL: &url.URL{Path: "/api/stacks/" + name + "/stop"}}
			HandleStopStack(writer, r)
		case "down", "rm", "remove":
			if len(args) < 3 {
				fmt.Fprintf(os.Stderr, "Usage: dc stack %s <name>\n", cmd)
				os.Exit(1)
			}
			name := args[2]
			r := &http.Request{Method: http.MethodDelete, URL: &url.URL{Path: "/api/stacks/" + name}}
			HandleDeleteStack(writer, r)
		case "logs":
			if len(args) < 3 {
				fmt.Fprintf(os.Stderr, "Usage: dc stack logs <name>\n")
				os.Exit(1)
			}
			name := args[2]
			r := &http.Request{Method: http.MethodGet, URL: &url.URL{Path: "/api/stacks/" + name + "/logs"}}
			HandleStreamStackLogs(writer, r)
		default:
			fmt.Fprintf(os.Stderr, "Unknown stack command: %s\n", cmd)
			os.Exit(1)
		}

	case "pw", "secret":
		// Forward pw/secret commands to an external `pw` script which reads/writes the env store.
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Usage: dc %s <args...>\n", args[0])
			os.Exit(1)
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
			fmt.Fprintf(os.Stderr, "pw command failed: %v\n", err)
			os.Exit(1)
		}

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", args[0])
		os.Exit(1)
	}
}

// findYAML searches common locations for a stack YAML file. Kept from the previous CLI logic.
func findYAML(name string) ([]byte, error) {
	home, _ := os.UserHomeDir()
	u := os.Getenv("USER")

	candidates := []string{
		fmt.Sprintf("./%s.yml", name),
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
