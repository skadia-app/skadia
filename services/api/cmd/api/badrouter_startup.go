package main

import (
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/routing"
)

type badRouterProfileStartupStatus struct {
	Configured bool
	Available  bool
	Fatal      bool
	Reason     string
	Error      string
	Count      int
}

func loadBadRouterProfilesForStartup(deploymentProfile, deploymentProfileSource, profileDir string) badRouterProfileStartupStatus {
	if deploymentProfile == "core" {
		count, err := routing.LoadBuiltinProfiles(profileDir)
		status := badRouterProfileStartupStatus{Configured: false, Available: false, Reason: "core_profile", Count: count}
		if err != nil {
			status.Error = err.Error()
		}
		return status
	}

	count, err := routing.LoadBuiltinProfiles(profileDir)
	status := badRouterProfileStartupStatus{
		Configured: deploymentProfileSource == "env",
		Available:  count > 0 && err == nil,
		Count:      count,
	}
	if deploymentProfileSource == "implicit_legacy" && count > 0 {
		status.Configured = true
	}
	if err != nil {
		status.Error = err.Error()
		status.Reason = badRouterProfileErrorReason(err.Error())
		return status
	}
	if count == 0 {
		status.Reason = "missing_profiles"
		return status
	}
	for _, name := range routing.ValidProfileNames {
		if _, builtinErr := routing.BuiltinProfile(name); builtinErr != nil {
			status.Available = false
			status.Reason = "missing_profiles"
			status.Error = builtinErr.Error()
			return status
		}
	}
	status.Configured = true
	status.Available = true
	return status
}

func badRouterProfileErrorReason(msg string) string {
	if strings.Contains(msg, "parse failed") || strings.Contains(msg, "validation failed") || strings.Contains(msg, "incomplete") || strings.Contains(msg, "profile field mismatch") {
		return "malformed_profiles"
	}
	return "missing_profiles"
}
