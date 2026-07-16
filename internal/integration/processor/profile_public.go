package processor

import (
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/profile"
)

// CompileProfileRevision applies the production profile compiler to one exact
// digest-bound profile revision for preview consumers outside this package.
func CompileProfileRevision(
	ref integration.ArtifactRevisionRef,
	raw []byte,
) (*profile.SourceProfile, *time.Location, error) {
	compiled, err := compileSourceProfile(ResolvedArtifactRevisions{
		profileRef:  ref,
		profileJSON: append([]byte(nil), raw...),
	})
	if err != nil {
		return nil, nil, err
	}
	return compiled.source, compiled.timezone, nil
}
