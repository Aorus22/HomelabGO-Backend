package system

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
)

type Service struct {
	Name        string `json:"name"`
	LoadState   string `json:"load_state"`
	ActiveState string `json:"active_state"`
	SubState    string `json:"sub_state"`
	Description string `json:"description"`
	Path        string `json:"path"` // Path to unit file, to identify if managed
}

type CronJob struct {
	ID       string `json:"id"` // Just for frontend key, we use index backend side effectively
	Schedule string `json:"schedule"`
	Command  string `json:"command"`
	Raw      string `json:"raw"` // Preserve original line for safe editing/identification
}

func GetCrontab() ([]CronJob, error) {
	cmd := exec.Command("crontab", "-l")
	out, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "no crontab") {
			return []CronJob{}, nil
		}
		// Some systems print to stderr but exit 0? No, standard error check.
		// If exit code is not 0, it returns error.
		return nil, fmt.Errorf("failed to get crontab: %v, output: %s", err, string(out))
	}

	var jobs []CronJob
	lines := strings.Split(string(out), "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		// Simple validation: Needs at least 5 fields or start with @
		fields := strings.Fields(trimmed)
		if len(fields) < 2 { // Invalid line?
			continue
		}

		job := CronJob{
			ID:  fmt.Sprintf("job-%d", i),
			Raw: line,
		}

		if strings.HasPrefix(trimmed, "@") {
			job.Schedule = fields[0]
			job.Command = strings.Join(fields[1:], " ")
		} else if len(fields) >= 6 { // 5 time fields + command
			job.Schedule = strings.Join(fields[:5], " ")
			job.Command = strings.Join(fields[5:], " ")
		} else {
			// Fallback: treat as raw/unknown
			job.Schedule = "???"
			job.Command = trimmed
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

func SaveCronJobs(jobs []CronJob) error {
	var lines []string
	// We might lose comments this way unless we store them.
	// For MVP, we rewrite the file with the jobs provided.
	lines = append(lines, "# Managed by HomelabGO Admin")
	for _, job := range jobs {
		line := fmt.Sprintf("%s %s", job.Schedule, job.Command)
		lines = append(lines, line)
	}
	// Ensure newline at end
	content := strings.Join(lines, "\n") + "\n"

	cmd := exec.Command("crontab", "-")
	cmd.Stdin = strings.NewReader(content)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to save crontab: %v, output: %s", err, string(out))
	}
	return nil
}

func ListServices() ([]Service, error) {
	// list-units --type=service --all --no-pager --no-legend
	cmd := exec.Command("systemctl", "list-units", "--type=service", "--all", "--no-pager", "--no-legend")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}

	// We also need to know the unit file path to determine if it is "managed" (user created in /etc/systemd/system)
	// systemctl show -p FragmentPath unit.service
	// Doing this for ALL services is slow (N calls).
	// Faster approach: List all files in /etc/systemd/system/ that end in .service
	// And mark those as managed in the list.

	managedMap := make(map[string]bool)
	// ls /etc/systemd/system/*.service
	// But this requires globbing.
	// Alternative: just assume services starting with "homelab-" or user provided convention?
	// Or we just fetch FragmentPath for the service when we need detail?
	// User wants TAB: System vs Managed.
	// Let's rely on FragmentPath. To optimize, maybe we only populate it on detail?
	// No, we need to filter list.
	// Optim: systemctl list-unit-files --state=enabled,disabled ...
	// creates output: unit.service related_path.

	// Better: systemctl list-units ... --output=json (available in newer systemd?)
	// If not, we scan /etc/systemd/system/ for our managed services.

	managedFiles, _ := exec.Command("sh", "-c", "ls /etc/systemd/system/*.service").Output()
	managedList := strings.Split(string(managedFiles), "\n")
	for _, m := range managedList {
		parts := strings.Split(m, "/")
		if len(parts) > 0 {
			name := parts[len(parts)-1]
			if name != "" {
				managedMap[name] = true
			}
		}
	}

	var services []Service
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 5 {
			continue
		}

		name := parts[0]
		path := "/lib/systemd/system/" + name // default guess
		if managedMap[name] {
			path = "/etc/systemd/system/" + name
		}

		s := Service{
			Name:        name,
			LoadState:   parts[1],
			ActiveState: parts[2],
			SubState:    parts[3],
			Description: strings.Join(parts[4:], " "),
			Path:        path,
		}
		services = append(services, s)
	}
	return services, nil
}

func ServiceAction(name, action string) error {
	validActions := map[string]bool{
		"start":   true,
		"stop":    true,
		"restart": true,
		"enable":  true,
		"disable": true,
	}
	if !validActions[action] {
		return fmt.Errorf("invalid action: %s", action)
	}

	cmd := exec.Command("systemctl", action, name)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to %s service %s: %v, output: %s", action, name, err, string(out))
	}
	return nil
}

func GetServiceLogs(name string) (string, error) {
	cmd := exec.Command("journalctl", "-u", name, "-n", "100", "--no-pager")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get logs: %v, output: %s", err, string(out))
	}
	return string(out), nil
}

type ServiceConfig struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	ExecStart   string `json:"exec_start"`
	Directory   string `json:"directory"`
	User        string `json:"user"`
	AutoStart   bool   `json:"auto_start"`
}

func CreateService(config ServiceConfig) error {
	if !strings.HasSuffix(config.Name, ".service") {
		config.Name += ".service"
	}
	// Sanitize name?
	if strings.Contains(config.Name, "/") {
		return fmt.Errorf("invalid service name")
	}

	content := fmt.Sprintf(`[Unit]
Description=%s
After=network.target

[Service]
Type=simple
User=%s
WorkingDirectory=%s
ExecStart=%s
Restart=always

[Install]
WantedBy=multi-user.target
`, config.Description, config.User, config.Directory, config.ExecStart)

	path := "/etc/systemd/system/" + config.Name

	// We need root permissions usually. Assuming backend runs as root or via sudo.
	// Go writing file might fail if not root.
	// Using sh -c to write might be safer if we assume sudo capability (not implemented here yet).
	// We'll write file directly.

	// Since we are running in simple user mode likely, this might fail without sudo.
	// But let's assume implementation.

	// We'll use a temporary file then move it with sudo mv?
	// For now, implementing direct write (assuming backend has permission or we instruct user).

	// Actually, easier to use 'saveFile' logic or exec "echo ... > file".
	// But echo with sudo: sudo bash -c "echo ... > file"

	// We'll just try to write file.

	// NOTE: Because of newline chars, shell command is tricky.
	// Construct command to write.

	// Using os.WriteFile is better if we have permission.
	// If we don't, we can try `sudo Tee`.

	cmd := exec.Command("sudo", "tee", path)
	cmd.Stdin = strings.NewReader(content)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create service file: %v, output: %s", err, string(out))
	}

	// Daemon reload
	exec.Command("sudo", "systemctl", "daemon-reload").Run()

	if config.AutoStart {
		exec.Command("sudo", "systemctl", "enable", config.Name).Run()
		exec.Command("sudo", "systemctl", "start", config.Name).Run()
	}

	return nil
}

func DeleteService(name string) error {
	if !strings.HasSuffix(name, ".service") {
		return fmt.Errorf("invalid service name")
	}
	// Stop and disable
	exec.Command("sudo", "systemctl", "stop", name).Run()
	exec.Command("sudo", "systemctl", "disable", name).Run()

	path := "/etc/systemd/system/" + name

	// Remove file
	cmd := exec.Command("sudo", "rm", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to delete service file: %v, output: %s", err, string(out))
	}

	exec.Command("sudo", "systemctl", "daemon-reload").Run()
	return nil
}

type OpenPort struct {
	Protocol string `json:"protocol"`
	Port     string `json:"port"`
	Address  string `json:"address"`
	Process  string `json:"process"`
	PID      string `json:"pid"`
}

func GetOpenPorts() ([]OpenPort, error) {
	// ss -tulpn or netstat -tulpn
	// using ss usually available on modern linux
	// output: Netid State Recv-Q Send-Q Local Address:Port Peer Address:Port Process
	// We use -H to suppress header
	cmd := exec.Command("sudo", "ss", "-tulpnH")
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Fallback to netstat?
		cmd = exec.Command("sudo", "netstat", "-tulpn")
		out, err = cmd.CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("failed to get ports: %v", err)
		}
	}

	var ports []OpenPort
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		// ss output format might vary slightly
		// Example ss: tcp LISTEN 0 128 0.0.0.0:22 0.0.0.0:* users:(("sshd",pid=123,fd=3))
		// NetidState ... Address:Port ... Process

		proto := fields[0]
		addr := fields[4] // Local Address:Port

		// Parse Address and Port
		// Address could be [::]:80 or 0.0.0.0:80 or *:80

		lastColon := strings.LastIndex(addr, ":")
		if lastColon == -1 {
			continue
		}

		portNum := addr[lastColon+1:]
		ipAddr := addr[:lastColon]

		// Process info usually in last field key=value
		// users:(("nginx",pid=123,fd=4))
		processRaw := fields[len(fields)-1]
		processName := "-"
		pid := "-"

		if strings.Contains(processRaw, "users:") {
			// Extract "nginx" and pid
			// This is fragile parsing but sufficient for MVP
			if start := strings.Index(processRaw, `("`); start != -1 {
				rem := processRaw[start+2:]
				if end := strings.Index(rem, `",`); end != -1 {
					processName = rem[:end]
				}
			}
			if start := strings.Index(processRaw, `pid=`); start != -1 {
				rem := processRaw[start+4:]
				// Read until non-digit
				end := 0
				for end < len(rem) && rem[end] >= '0' && rem[end] <= '9' {
					end++
				}
				pid = rem[:end]
			}
		}

		ports = append(ports, OpenPort{
			Protocol: proto,
			Port:     portNum,
			Address:  ipAddr,
			Process:  processName,
			PID:      pid,
		})
	}
	return ports, nil
}

type NetworkInterface struct {
	Name  string   `json:"name"`
	MAC   string   `json:"mac"`
	IPs   []string `json:"ips"`
	Flags string   `json:"flags"`
	MTU   int      `json:"mtu"`
}

func GetNetworkInterfaces() ([]NetworkInterface, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	var result []NetworkInterface
	for _, i := range ifaces {
		var ips []string
		addrs, _ := i.Addrs()
		for _, addr := range addrs {
			ips = append(ips, addr.String())
		}

		result = append(result, NetworkInterface{
			Name:  i.Name,
			MAC:   i.HardwareAddr.String(),
			IPs:   ips,
			Flags: i.Flags.String(),
			MTU:   i.MTU,
		})
	}
	return result, nil
}

type Process struct {
	PID     string `json:"pid"`
	User    string `json:"user"`
	CPU     string `json:"cpu"`
	Memory  string `json:"memory"`
	Command string `json:"command"`
}

func GetProcesses() ([]Process, error) {
	// ps -eo pid,user,%cpu,%mem,comm --sort=-%cpu | head -n 50
	cmd := exec.Command("ps", "-eo", "pid,user,%cpu,%mem,comm", "--sort=-%cpu")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to get processes: %v", err)
	}

	var processes []Process
	lines := strings.Split(string(out), "\n")

	// Skip header (first line)
	if len(lines) > 0 {
		lines = lines[1:]
	}

	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}

		// fields: PID USER %CPU %MEM COMMAND
		// COMMAND might contain spaces if we didn't use 'comm', but 'comm' usually truncated
		// Using 'comm' (command name only) is safer for parsing than 'args'

		processes = append(processes, Process{
			PID:     fields[0],
			User:    fields[1],
			CPU:     fields[2],
			Memory:  fields[3],
			Command: strings.Join(fields[4:], " "), // In case comm has spaces
		})

		// Limit to top 50
		if len(processes) >= 50 {
			break
		}
	}
	return processes, nil
}

func KillProcess(pid string) error {
	// Safety check: don't allow killing init or self easily via API if possible,
	// but for now relying on sudo/user permissions.
	// Using sudo kill -9
	cmd := exec.Command("sudo", "kill", "-9", pid)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to kill process %s: %v, output: %s", pid, err, string(out))
	}
	return nil
}

type FirewallRule struct {
	Index   string `json:"index"`
	To      string `json:"to"`
	Action  string `json:"action"`
	From    string `json:"from"`
	Comment string `json:"comment"`
}

type FirewallStatus struct {
	Status string         `json:"status"` // active, inactive
	Rules  []FirewallRule `json:"rules"`
}

func GetFirewallStatus() (*FirewallStatus, error) {
	cmd := exec.Command("sudo", "ufw", "status", "numbered")
	out, err := cmd.CombinedOutput()
	// ufw exits with 0 even if inactive, but output says "Status: inactive"
	// if command fails (e.g. not installed), we assume inactive or error.

	output := string(out)
	status := "inactive"
	if strings.Contains(output, "Status: active") {
		status = "active"
	}

	var rules []FirewallRule
	if status == "active" {
		lines := strings.Split(output, "\n")
		// Output format:
		// Status: active
		//
		//      To                         Action      From
		//      --                         ------      ----
		// [ 1] 22/tcp                     ALLOW IN    Anywhere
		// [ 2] 80                         ALLOW IN    Anywhere

		for _, line := range lines {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "[") {
				continue
			}

			// Simple parsing based on brackets
			// [ 1] 22/tcp                     ALLOW IN    Anywhere

			closeBracket := strings.Index(line, "]")
			if closeBracket == -1 {
				continue
			}

			index := strings.TrimSpace(line[1:closeBracket])
			rest := line[closeBracket+1:]

			fields := strings.Fields(rest)
			// fields: To, Action(ALLOW, IN...), From...

			if len(fields) >= 3 {
				to := fields[0]

				// Find where "Action" ends (ALLOW IN, DENY IN, ALLOW OUT, etc)
				// Let's assume the last part is "From" (maybe multiple words if (v6))
				// But "From" usually starts after Action.
				// This simple parser might be fragile.

				action := fields[1]
				if len(fields) > 2 && (fields[2] == "IN" || fields[2] == "OUT" || fields[2] == "FWD") {
					action = fields[1] + " " + fields[2]
				}

				// Reconstruct From
				// Find where Action ends in the substring
				actionIndex := strings.Index(rest, action)
				if actionIndex != -1 {
					fromPart := strings.TrimSpace(rest[actionIndex+len(action):])
					rules = append(rules, FirewallRule{
						Index:  index,
						To:     to,
						Action: action,
						From:   fromPart,
					})
				}
			}
		}
	}

	if err != nil && !strings.Contains(output, "Status:") {
		// Real error, maybe ufw not installed
		return nil, fmt.Errorf("failed to get ufw status: %v", err)
	}

	return &FirewallStatus{
		Status: status,
		Rules:  rules,
	}, nil
}

func ToggleFirewall(enable bool) error {
	action := "disable"
	if enable {
		action = "enable"
	}
	// ufw enable requires 'y' confirmation usually, so use --force or echo y
	// sudo ufw --force enable
	var cmd *exec.Cmd
	if enable {
		cmd = exec.Command("sudo", "ufw", "--force", "enable")
	} else {
		cmd = exec.Command("sudo", "ufw", "disable")
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to %s firewall: %v, output: %s", action, err, string(out))
	}
	return nil
}

func AddFirewallRule(port, proto, action string) error {
	// sudo ufw allow 80/tcp
	// action: allow, deny
	if action != "allow" && action != "deny" {
		return fmt.Errorf("invalid action")
	}

	target := port
	if proto != "" && proto != "any" {
		target = fmt.Sprintf("%s/%s", port, proto)
	}

	cmd := exec.Command("sudo", "ufw", action, target)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to add rule: %v, output: %s", err, string(out))
	}
	return nil
}

func DeleteFirewallRule(index string) error {
	// sudo ufw --force delete <index>
	// --force prevents confirmation prompt "Delete rule 1 (y|n)?"
	cmd := exec.Command("sudo", "ufw", "--force", "delete", index)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete rule: %v, output: %s", err, string(out))
	}
	return nil
}
