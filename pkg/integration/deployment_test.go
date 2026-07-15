package integration_test

import (
	"errors"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

func validDeploymentPolicy() integration.IntegrationDeploymentPolicy {
	return integration.IntegrationDeploymentPolicy{
		ConnectionValidation: integration.ConnectionValidationPolicy{TimeoutSeconds: 10, MaxAgeSeconds: 300},
		Schedule:             integration.SchedulePolicy{Mode: integration.ScheduleModeContinuous},
		Health: integration.HealthPolicy{
			StartupGraceSeconds:  30,
			CheckIntervalSeconds: 15,
			TimeoutSeconds:       5,
			FailureThreshold:     3,
		},
		Capacity: integration.CapacityPolicy{
			MaxInFlight:          32,
			MaxQueued:            1024,
			MaxMessagesPerSecond: 250,
		},
	}
}

func TestDeploymentPolicySupportsContinuousAndCronSchedules(t *testing.T) {
	continuous := validDeploymentPolicy()
	if err := continuous.Validate(); err != nil {
		t.Fatalf("continuous policy: %v", err)
	}
	cron := validDeploymentPolicy()
	cron.Schedule = integration.SchedulePolicy{
		Mode:           integration.ScheduleModeCron,
		CronExpression: "0 2 * * *",
		Timezone:       "America/New_York",
	}
	if err := cron.Validate(); err != nil {
		t.Fatalf("cron policy: %v", err)
	}
}

func TestDeploymentPolicyRejectsUnsafeBounds(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*integration.IntegrationDeploymentPolicy)
	}{
		{name: "validation timeout", mutate: func(p *integration.IntegrationDeploymentPolicy) { p.ConnectionValidation.TimeoutSeconds = 0 }},
		{name: "validation age", mutate: func(p *integration.IntegrationDeploymentPolicy) { p.ConnectionValidation.MaxAgeSeconds = 5 }},
		{name: "schedule", mutate: func(p *integration.IntegrationDeploymentPolicy) { p.Schedule.Mode = "sometimes" }},
		{name: "continuous cron", mutate: func(p *integration.IntegrationDeploymentPolicy) { p.Schedule.CronExpression = "* * * * *" }},
		{name: "cron timezone", mutate: func(p *integration.IntegrationDeploymentPolicy) {
			p.Schedule = integration.SchedulePolicy{Mode: integration.ScheduleModeCron, CronExpression: "* * * * *", Timezone: "Mars/Olympus"}
		}},
		{name: "health timeout", mutate: func(p *integration.IntegrationDeploymentPolicy) {
			p.Health.TimeoutSeconds = p.Health.CheckIntervalSeconds
		}},
		{name: "health failures", mutate: func(p *integration.IntegrationDeploymentPolicy) { p.Health.FailureThreshold = 0 }},
		{name: "in flight", mutate: func(p *integration.IntegrationDeploymentPolicy) { p.Capacity.MaxInFlight = 0 }},
		{name: "queue", mutate: func(p *integration.IntegrationDeploymentPolicy) { p.Capacity.MaxQueued = p.Capacity.MaxInFlight - 1 }},
		{name: "rate", mutate: func(p *integration.IntegrationDeploymentPolicy) { p.Capacity.MaxMessagesPerSecond = 0 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := validDeploymentPolicy()
			tt.mutate(&policy)
			if err := policy.Validate(); err == nil {
				t.Fatal("expected invalid deployment policy")
			}
		})
	}
}

func TestDeployableRevisionIsContentAddressedAndDefensivelyCopied(t *testing.T) {
	input := validRevisionInput()
	policy := validDeploymentPolicy()
	input.Deployment = &policy
	revision, err := integration.NewIntegrationDefinitionRevision(input)
	if err != nil {
		t.Fatalf("construct deployable revision: %v", err)
	}
	if err := revision.ValidateForDeployment(); err != nil {
		t.Fatalf("validate deployable revision: %v", err)
	}

	policy.Capacity.MaxQueued = 999999
	if revision.Deployment.Capacity.MaxQueued != 1024 {
		t.Fatal("constructor retained caller deployment-policy pointer")
	}
	revision.Deployment.Capacity.MaxQueued++
	var validationErr *integration.ValidationError
	if err := revision.Validate(); !errors.As(err, &validationErr) {
		t.Fatalf("deployment mutation did not invalidate digest: %v", err)
	}
}

func TestLegacyRevisionDigestRemainsStableWithoutDeploymentPolicy(t *testing.T) {
	revision, err := integration.NewIntegrationDefinitionRevision(validRevisionInput())
	if err != nil {
		t.Fatalf("construct legacy revision: %v", err)
	}
	const want = "sha256:b0de3dec889161d4af95f2bc51fa77bbb6b2adababd4eb7dd491fbec60fbc925"
	if revision.Digest != want {
		t.Fatalf("legacy digest changed: got %s want %s", revision.Digest, want)
	}
	if err := revision.ValidateForDeployment(); err == nil {
		t.Fatal("legacy revision unexpectedly passed deployment validation")
	}
}
