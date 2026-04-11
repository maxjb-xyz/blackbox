package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"slices"
	"sort"
	"strings"
	"syscall"
	"time"

	"blackbox/agent/internal/client"
	"blackbox/agent/internal/docker"
	"blackbox/agent/internal/files"
	"blackbox/agent/internal/queue"
	"blackbox/agent/internal/sender"
	"blackbox/agent/internal/systemd"
	"blackbox/shared/timezone"
	"blackbox/shared/types"
	"github.com/oklog/ulid/v2"
)

var (
	Version = "dev"
	Commit  = "unknown"
)

func main() {
	if tz, err := timezone.ConfigureLocal(); err != nil {
		log.Printf("timezone: invalid TZ %q: %v; using container default timezone", tz, err)
	}
	log.Printf("Blackbox Agent %s (%s) starting", Version, Commit)

	serverURL := mustEnv("SERVER_URL")
	agentToken := mustEnv("AGENT_TOKEN")

	nodeName := os.Getenv("NODE_NAME")
	if nodeName == "" {
		var err error
		nodeName, err = os.Hostname()
		if err != nil {
			nodeName = "unknown"
		}
	}
	info := collectNodeInfo(serverURL)
	infoJSON, err := json.Marshal(info)
	if err != nil {
		log.Printf("WARNING: failed to marshal node info: %v", err)
		infoJSON = []byte("{}")
	}

	watchPaths := splitEnv("WATCH_PATHS")
	watchIgnore := splitEnv("WATCH_IGNORE")

	queueDBPath := os.Getenv("QUEUE_DB_PATH")
	if queueDBPath == "" {
		queueDBPath = "/data/queue.db"
	}
	q, err := queue.New(queueDBPath)
	if err != nil {
		log.Fatalf("queue: failed to open persistent queue at %s: %v", queueDBPath, err)
	}
	defer func() {
		if err := q.Close(); err != nil {
			log.Printf("queue: close error: %v", err)
		}
	}()
	swept, err := q.SweepStale(7 * 24 * time.Hour)
	if err != nil {
		log.Printf("queue: stale sweep failed: %v", err)
	} else if swept > 0 {
		log.Printf("queue: swept %d stale entries (older than 7 days)", swept)
	}

	c := client.New(serverURL, agentToken, nodeName)
	s := sender.New(c, q)
	out := s.Chan()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go s.Start(ctx)
	go docker.Watch(ctx, nodeName, out)

	if len(watchPaths) > 0 {
		fileWatcherSettings := files.NewSettings(loadFileWatcherRedactSecrets(ctx, c))
		go refreshFileWatcherSettings(ctx, c, fileWatcherSettings)
		count := files.Watch(ctx, nodeName, watchPaths, watchIgnore, fileWatcherSettings, out)
		log.Printf("file watcher: watching %d directories across %d root paths", count, len(watchPaths))
		if count == 0 {
			log.Printf("file watcher: WARNING — no directories registered; check WATCH_PATHS and system max_user_watches")
		}
	} else {
		log.Println("file watcher: WATCH_PATHS not set, file watching disabled")
	}

	if os.Getenv("WATCH_SYSTEMD") == "true" {
		if !systemd.Supported() {
			log.Println("systemd watcher: disabled in this build; rebuild on Linux with cgo, libsystemd headers, and -tags systemd to enable")
		} else {
			initialUnits := loadSystemdUnits(ctx, c)
			systemdSettings := systemd.NewSettings(initialUnits)
			go refreshSystemdSettings(ctx, c, systemdSettings)
			go systemd.Watch(ctx, nodeName, systemdSettings, out)
			log.Printf("systemd watcher: started, watching %d units", len(initialUnits))
		}
	} else {
		log.Println("systemd watcher: WATCH_SYSTEMD not set, systemd watching disabled")
	}

	out <- types.Entry{
		ID:        ulid.Make().String(),
		Timestamp: time.Now().UTC(),
		NodeName:  nodeName,
		Source:    "agent",
		Event:     "start",
		Content:   "Blackbox Agent started",
		Metadata:  string(infoJSON),
	}

	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				out <- types.Entry{
					ID:        ulid.Make().String(),
					Timestamp: time.Now().UTC(),
					NodeName:  nodeName,
					Source:    "agent",
					Event:     "heartbeat",
					Content:   "Blackbox Agent heartbeat",
					Metadata:  string(infoJSON),
				}
			}
		}
	}()

	log.Printf("startup heartbeat sent (node: %s)", nodeName)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down...")
	out <- types.Entry{
		ID:        ulid.Make().String(),
		Timestamp: time.Now().UTC(),
		NodeName:  nodeName,
		Source:    "agent",
		Event:     "shutdown",
		Content:   "Blackbox Agent shutting down",
		Metadata:  string(infoJSON),
	}
	cancel()
	<-s.Done()
	log.Println("shutdown complete")
}

func loadFileWatcherRedactSecrets(ctx context.Context, c *client.Client) bool {
	configCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	config, err := c.GetAgentConfig(configCtx)
	if err != nil {
		log.Printf("file watcher: failed to load server config, defaulting redact_secrets=true: %v", err)
		return true
	}
	return config.FileWatcherRedactSecrets
}

func refreshFileWatcherSettings(ctx context.Context, c *client.Client, settings *files.Settings) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			redactSecrets := loadFileWatcherRedactSecrets(ctx, c)
			if settings.RedactSecrets() != redactSecrets {
				log.Printf("file watcher: updated redact_secrets=%t from server config", redactSecrets)
			}
			settings.SetRedactSecrets(redactSecrets)
		}
	}
}

func loadSystemdUnits(ctx context.Context, c *client.Client) []string {
	configCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	config, err := c.GetAgentConfig(configCtx)
	if err != nil {
		log.Printf("systemd watcher: failed to load units from server, starting with empty list: %v", err)
		return nil
	}
	return config.SystemdUnits
}

func refreshSystemdSettings(ctx context.Context, c *client.Client, settings *systemd.Settings) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	prevUnits := settings.Units()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			newUnits := loadSystemdUnits(ctx, c)
			if !slices.Equal(prevUnits, newUnits) {
				log.Printf("systemd watcher: refreshed units (%d total, added: %s, removed: %s)", len(newUnits), summarizeUnitDiff(newUnits, prevUnits), summarizeUnitDiff(prevUnits, newUnits))
			}
			settings.SetUnits(newUnits)
			prevUnits = newUnits
		}
	}
}

func summarizeUnitDiff(current, previous []string) string {
	var changed []string
	for _, unit := range current {
		if !slices.Contains(previous, unit) {
			changed = append(changed, unit)
		}
	}
	if len(changed) == 0 {
		return "none"
	}
	if len(changed) > 3 {
		return strings.Join(changed[:3], ", ") + ", ..."
	}
	return strings.Join(changed, ", ")
}

type nodeInfo struct {
	AgentVersion string `json:"agent_version"`
	IPAddress    string `json:"ip_address"`
	OsInfo       string `json:"os_info"`
}

func collectNodeInfo(serverURL string) nodeInfo {
	ipAddress, err := getServerReachableIP(serverURL)
	if err != nil {
		log.Printf("node info: failed to resolve local IP for SERVER_URL %q: %v", serverURL, err)
		ipAddress = getOutboundIP()
	}

	return nodeInfo{
		AgentVersion: Version,
		IPAddress:    ipAddress,
		OsInfo:       getOSInfo(),
	}
}

func getOSInfo() string {
	_, inDocker := os.Stat("/.dockerenv")
	isDocker := inDocker == nil

	osName := readOSReleaseName()
	if isDocker {
		return "docker / " + osName
	}
	return osName
}

func readOSReleaseName() string {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return runtime.GOOS
	}
	lines := strings.Split(string(data), "\n")
	for _, prefix := range []string{"PRETTY_NAME=", "NAME="} {
		for _, line := range lines {
			if strings.HasPrefix(line, prefix) {
				value := strings.TrimSpace(strings.Trim(strings.TrimPrefix(line, prefix), `"`))
				if value == "" {
					continue
				}
				return value
			}
		}
	}
	return runtime.GOOS
}

func getServerReachableIP(serverURL string) (string, error) {
	target, err := serverDialTarget(serverURL)
	if err != nil {
		return "", err
	}

	conn, err := net.DialTimeout("udp", target, 5*time.Second)
	if err != nil {
		conn, err = net.DialTimeout("tcp", target, 5*time.Second)
		if err != nil {
			return "", fmt.Errorf("dial %s: %w", target, err)
		}
	}
	defer func() {
		_ = conn.Close()
	}()

	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if ok && addr.IP != nil {
		return addr.IP.String(), nil
	}

	host, _, err := net.SplitHostPort(conn.LocalAddr().String())
	if err != nil {
		return "", fmt.Errorf("parse local address %q: %w", conn.LocalAddr().String(), err)
	}
	return host, nil
}

func serverDialTarget(serverURL string) (string, error) {
	parsed, err := url.Parse(serverURL)
	if err != nil || parsed.Host == "" {
		parsed, err = url.Parse("http://" + serverURL)
		if err != nil {
			return "", fmt.Errorf("parse server url %q: %w", serverURL, err)
		}
	}

	host := parsed.Hostname()
	if host == "" {
		return "", fmt.Errorf("server url %q is missing a host", serverURL)
	}

	port := parsed.Port()
	if port == "" {
		switch parsed.Scheme {
		case "https", "wss":
			port = "443"
		default:
			port = "80"
		}
	}

	return net.JoinHostPort(host, port), nil
}

func getOutboundIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	sort.Slice(ifaces, func(i, j int) bool { return ifaces[i].Name < ifaces[j].Name })
	dockerPrefixes := []string{"172.17.", "172.18.", "172.19.", "172.20.", "172.21.", "172.22.", "172.23.", "172.24."}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			ip4 := ip.To4()
			if ip4 == nil || ip4.IsLoopback() {
				continue
			}
			ipStr := ip4.String()
			skip := false
			for _, prefix := range dockerPrefixes {
				if strings.HasPrefix(ipStr, prefix) {
					skip = true
					break
				}
			}
			if !skip {
				return ipStr
			}
		}
	}
	return ""
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("%s environment variable is required", key)
	}
	return v
}

func splitEnv(key string) []string {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ":")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
