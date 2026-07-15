package integration

import (
	"strings"
	"time"
)

// DeploymentState is the durable lifecycle state of one immutable revision.
type DeploymentState string

const (
	DeploymentStateDraft     DeploymentState = "draft"
	DeploymentStateValidated DeploymentState = "validated"
	DeploymentStateApproved  DeploymentState = "approved"
	DeploymentStatePublished DeploymentState = "published"
	DeploymentStateDeployed  DeploymentState = "deployed"
	DeploymentStatePaused    DeploymentState = "paused"
	DeploymentStateRetired   DeploymentState = "retired"
)

// DeploymentHealthStatus is the latest safe operational health projection.
type DeploymentHealthStatus string

const (
	DeploymentHealthUnknown   DeploymentHealthStatus = "unknown"
	DeploymentHealthStarting  DeploymentHealthStatus = "starting"
	DeploymentHealthHealthy   DeploymentHealthStatus = "healthy"
	DeploymentHealthDegraded  DeploymentHealthStatus = "degraded"
	DeploymentHealthUnhealthy DeploymentHealthStatus = "unhealthy"
)

// ScheduleMode selects continuous operation or a five-field cron schedule.
type ScheduleMode string

const (
	ScheduleModeContinuous ScheduleMode = "continuous"
	ScheduleModeCron       ScheduleMode = "cron"
)

// ConnectionValidationPolicy bounds validation work and evidence freshness.
type ConnectionValidationPolicy struct {
	TimeoutSeconds int64 `json:"timeout_seconds"`
	MaxAgeSeconds  int64 `json:"max_age_seconds"`
}

// SchedulePolicy controls when a deployed source is allowed to run.
type SchedulePolicy struct {
	Mode           ScheduleMode `json:"mode"`
	CronExpression string       `json:"cron_expression,omitempty"`
	Timezone       string       `json:"timezone,omitempty"`
}

// HealthPolicy controls startup grace and consecutive check failure handling.
type HealthPolicy struct {
	StartupGraceSeconds  int64 `json:"startup_grace_seconds"`
	CheckIntervalSeconds int64 `json:"check_interval_seconds"`
	TimeoutSeconds       int64 `json:"timeout_seconds"`
	FailureThreshold     int   `json:"failure_threshold"`
}

// CapacityPolicy bounds concurrency, queued work, and accepted message rate.
type CapacityPolicy struct {
	MaxInFlight          int `json:"max_in_flight"`
	MaxQueued            int `json:"max_queued"`
	MaxMessagesPerSecond int `json:"max_messages_per_second"`
}

// IntegrationDeploymentPolicy is immutable runtime policy for one revision.
type IntegrationDeploymentPolicy struct {
	ConnectionValidation ConnectionValidationPolicy `json:"connection_validation"`
	Schedule             SchedulePolicy             `json:"schedule"`
	Health               HealthPolicy               `json:"health"`
	Capacity             CapacityPolicy             `json:"capacity"`
}

// Validate verifies deployment policy without performing external I/O.
func (p IntegrationDeploymentPolicy) Validate() error {
	v := &validationCollector{}
	v.add(p.ConnectionValidation.TimeoutSeconds > 0 && p.ConnectionValidation.TimeoutSeconds <= 300,
		"OUT_OF_RANGE", "connection_validation.timeout_seconds", "connection validation timeout must be between 1 and 300 seconds")
	v.add(p.ConnectionValidation.MaxAgeSeconds >= p.ConnectionValidation.TimeoutSeconds && p.ConnectionValidation.MaxAgeSeconds <= 86400,
		"OUT_OF_RANGE", "connection_validation.max_age_seconds", "connection validation max age must cover the timeout and be at most 86400 seconds")
	validateSchedulePolicy(p.Schedule, v)
	v.add(p.Health.StartupGraceSeconds >= 0 && p.Health.StartupGraceSeconds <= 3600,
		"OUT_OF_RANGE", "health.startup_grace_seconds", "health startup grace must be between 0 and 3600 seconds")
	v.add(p.Health.CheckIntervalSeconds > 0 && p.Health.CheckIntervalSeconds <= 300,
		"OUT_OF_RANGE", "health.check_interval_seconds", "health check interval must be between 1 and 300 seconds")
	v.add(p.Health.TimeoutSeconds > 0 && p.Health.TimeoutSeconds < p.Health.CheckIntervalSeconds,
		"OUT_OF_RANGE", "health.timeout_seconds", "health timeout must be positive and shorter than the check interval")
	v.add(p.Health.FailureThreshold > 0 && p.Health.FailureThreshold <= 10,
		"OUT_OF_RANGE", "health.failure_threshold", "health failure threshold must be between 1 and 10")
	v.add(p.Capacity.MaxInFlight > 0 && p.Capacity.MaxInFlight <= 10000,
		"OUT_OF_RANGE", "capacity.max_in_flight", "maximum in-flight work must be between 1 and 10000")
	v.add(p.Capacity.MaxQueued >= p.Capacity.MaxInFlight && p.Capacity.MaxQueued <= 1000000,
		"OUT_OF_RANGE", "capacity.max_queued", "maximum queued work must cover in-flight work and be at most 1000000")
	v.add(p.Capacity.MaxMessagesPerSecond > 0 && p.Capacity.MaxMessagesPerSecond <= 1000000,
		"OUT_OF_RANGE", "capacity.max_messages_per_second", "maximum message rate must be between 1 and 1000000 per second")
	return v.err()
}

func validateSchedulePolicy(policy SchedulePolicy, v *validationCollector) {
	switch policy.Mode {
	case ScheduleModeContinuous:
		v.add(strings.TrimSpace(policy.CronExpression) == "", "FORBIDDEN", "schedule.cron_expression", "continuous schedules cannot set a cron expression")
		v.add(strings.TrimSpace(policy.Timezone) == "", "FORBIDDEN", "schedule.timezone", "continuous schedules cannot set a timezone")
	case ScheduleModeCron:
		v.add(len(strings.Fields(policy.CronExpression)) == 5, "INVALID_CRON", "schedule.cron_expression", "cron schedules require a five-field expression")
		if strings.TrimSpace(policy.Timezone) == "" {
			v.add(false, "REQUIRED", "schedule.timezone", "cron schedules require an IANA timezone")
		} else if _, err := time.LoadLocation(policy.Timezone); err != nil {
			v.add(false, "INVALID_TIMEZONE", "schedule.timezone", "cron schedule timezone must be a known IANA location")
		}
	default:
		v.add(false, "INVALID_SCHEDULE_MODE", "schedule.mode", "schedule mode must be continuous or cron")
	}
}

// ValidateForDeployment requires the additive policy used by the lifecycle catalog.
func (r IntegrationDefinitionRevision) ValidateForDeployment() error {
	v := &validationCollector{}
	v.merge("", r.Validate())
	if r.Deployment == nil {
		v.add(false, "REQUIRED", "deployment", "deployment policy is required")
	} else {
		v.merge("deployment", r.Deployment.Validate())
	}
	return v.err()
}

func cloneDeploymentPolicy(policy *IntegrationDeploymentPolicy) *IntegrationDeploymentPolicy {
	if policy == nil {
		return nil
	}
	cloned := *policy
	return &cloned
}
