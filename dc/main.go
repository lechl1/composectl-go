package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

func main() {
	// Initialize paths first (respects --stacks-dir and --env-path arguments)
	InitPaths(os.Args)

	host := flag.String("host", "", "Server host (default: http://localhost:8882)")
	user := flag.String("user", "", "Username")
	password := flag.String("password", "", "Password")
	flag.Parse()

	args := flag.Args()
	if len(args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: dc [--host=...] [--user=...] [--password=...] <command> <name>\n")
		fmt.Fprintf(os.Stderr, "Commands: up, down, stop, logs, enrich\n")
		os.Exit(1)
	}
	command := args[0]
	name := args[1]

	if *host == "" {
		if v := os.Getenv("DC_HOST"); v != "" {
			*host = v
		} else {
			*host = "http://localhost:8882"
		}
	}
	if *user == "" {
		*user = os.Getenv("DC_USER")
	}
	if *password == "" {
		*password = os.Getenv("DC_PASSWORD")
	}

	token, err := login(*host, *user, *password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "login failed: %v\n", err)
		os.Exit(1)
	}

	var yamlBody []byte
	switch command {
	case "up", "down", "stop", "enrich":
		yamlBody, err = findYAML(name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
	case "logs":
		// no body needed
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", command)
		os.Exit(1)
	}

	if err := run(*host, token, command, name, yamlBody); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func login(host, user, password string) (string, error) {
	req, err := http.NewRequest(http.MethodPost, host+"/api/auth/login", nil)
	if err != nil {
		return "", err
	}
	creds := base64.StdEncoding.EncodeToString([]byte(user + ":" + password))
	req.Header.Set("Authorization", "Basic "+creds)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("login HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("parse login response: %w", err)
	}
	return result.Token, nil
}

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

func run(host, token, command, name string, body []byte) error {
	var method, url string
	switch command {
	case "up":
		method = http.MethodPut
		url = fmt.Sprintf("%s/api/stacks/%s/start", host, name)
	case "down":
		method = http.MethodDelete
		url = fmt.Sprintf("%s/api/stacks/%s", host, name)
	case "stop":
		method = http.MethodPut
		url = fmt.Sprintf("%s/api/stacks/%s/stop", host, name)
	case "logs":
		method = http.MethodGet
		url = fmt.Sprintf("%s/api/stacks/%s/logs", host, name)
	case "enrich":
		method = http.MethodPost
		url = fmt.Sprintf("%s/api/stacks/%s/enrich", host, name)
	}

	var bodyReader io.Reader
	if len(body) > 0 {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if len(body) > 0 {
		req.Header.Set("Content-Type", "text/plain")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if _, err := io.Copy(os.Stdout, resp.Body); err != nil {
		return err
	}
	return nil
}
