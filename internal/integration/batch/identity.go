package batch

import (
	"sort"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// Authentication method names recorded on the batch service principal.
const (
	// AuthMethodConnectorPrefix attributes a source to its deployment-fixed
	// connector principal. The provider name is appended.
	AuthMethodConnectorPrefix = "batch-"
	// AuthMethodWorkloadIdentity attributes a source to the canonical workload
	// subject declared by its immutable source revision.
	AuthMethodWorkloadIdentity = "batch-workload-identity"
)

const maxWorkloadGrants = 16

// WorkloadIdentity binds one batch source to one canonical service subject.
// The subject and its grants are deployment configuration carried inside the
// content-addressed source revision. Nothing observed on the remote side —
// object keys, remote metadata, or message content — can select or influence
// them.
type WorkloadIdentity struct {
	Subject string   `json:"subject"`
	Grants  []string `json:"grants,omitempty"`
}

// WorkloadIdentityEnabled reports whether this source submits under a declared
// workload subject. Binding is all-or-nothing per source: a source that
// declares a subject never falls back to the deployment-fixed principal.
func (r SourceRevision) WorkloadIdentityEnabled() bool {
	return r.Workload != nil
}

// resolvePrincipal converts the source-declared workload identity into
// server-owned principal fields. It fails closed rather than degrading a bound
// source to the deployment-fixed compatibility principal.
func (r *Runner) resolvePrincipal(sourceID string) (integration.Principal, error) {
	if !validIdentity(sourceID) {
		return integration.Principal{}, ErrUnavailable
	}
	if !r.source.WorkloadIdentityEnabled() {
		return integration.Principal{
			ID: r.principalID, Kind: integration.PrincipalKindService,
			AuthMethod: AuthMethodConnectorPrefix + string(r.source.Provider),
			Roles:      []string{SubmitRole}, SourceID: sourceID,
		}, nil
	}
	workload := *r.source.Workload
	if !validIdentity(workload.Subject) {
		return integration.Principal{}, ErrUnavailable
	}
	return integration.Principal{
		ID: workload.Subject, Kind: integration.PrincipalKindService,
		AuthMethod: AuthMethodWorkloadIdentity,
		Roles:      append([]string(nil), workload.Grants...), SourceID: sourceID,
	}, nil
}

func cloneWorkloadIdentity(workload *WorkloadIdentity) *WorkloadIdentity {
	if workload == nil {
		return nil
	}
	clone := *workload
	clone.Grants = append([]string(nil), workload.Grants...)
	return &clone
}

func canonicalWorkloadIdentity(workload *WorkloadIdentity) *WorkloadIdentity {
	clone := cloneWorkloadIdentity(workload)
	if clone == nil {
		return nil
	}
	sort.Strings(clone.Grants)
	return clone
}

func validateWorkloadIdentity(workload *WorkloadIdentity) error {
	if workload == nil {
		return nil
	}
	if !validIdentity(workload.Subject) || len(workload.Grants) > maxWorkloadGrants {
		return ErrInvalidSourceRevision
	}
	seen := make(map[string]struct{}, len(workload.Grants))
	for _, grant := range workload.Grants {
		if !validIdentity(grant) {
			return ErrInvalidSourceRevision
		}
		if _, duplicate := seen[grant]; duplicate {
			return ErrInvalidSourceRevision
		}
		seen[grant] = struct{}{}
	}
	return nil
}
