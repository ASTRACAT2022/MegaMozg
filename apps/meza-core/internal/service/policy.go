package service

import "strings"

func requiresApproval(jobType string) bool {
	switch strings.ToLower(jobType) {
	case "update_docker", "service_restart", "run_script", "self_update_agent", "shell_command", "bash_script":
		return true
	default:
		return false
	}
}
