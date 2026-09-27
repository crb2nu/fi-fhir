package connection

// HTTPSpec is the `http` connection spec: HTTPSourceRevisionInput minus the
// identifiers the catalog assigns. path defaults to /v1/hl7v2; within oauth,
// the three claims and the algorithm list default to what the runtime uses
// when their FI_FHIR_HTTP_INGRESS_OAUTH_* variable is unset.
type HTTPSpec struct {
	SourceID          string         `json:"source_id"`
	Path              string         `json:"path,omitempty"`
	AuthMode          string         `json:"auth_mode"`
	PrincipalID       string         `json:"principal_id,omitempty"`
	CredentialBinding string         `json:"credential_binding,omitempty"`
	OAuth             *HTTPOAuthSpec `json:"oauth,omitempty"`
	MaxBodyBytes      *int64         `json:"max_body_bytes"`
}

// HTTPOAuthSpec is HTTPOAuthPolicy with optional claims and algorithms.
type HTTPOAuthSpec struct {
	IssuerURL        string   `json:"issuer_url"`
	Audience         string   `json:"audience"`
	TenantClaim      string   `json:"tenant_claim,omitempty"`
	RolesClaim       string   `json:"roles_claim,omitempty"`
	ClientIDClaim    string   `json:"client_id_claim,omitempty"`
	SigningAlgs      []string `json:"signing_algs,omitempty"`
	AllowedClientIDs []string `json:"allowed_client_ids"`
}

func (s *HTTPSpec) check(c *checker) {
	c.identity("source_id", s.SourceID)
	if s.Path != "" && !validHTTPSourcePath(s.Path) {
		c.add(CodeInvalidValue, "path", "must be a clean absolute URL path such as /v1/hl7v2")
	}
	c.intRange("max_body_bytes", s.MaxBodyBytes, 1, maxHTTPSourceBodyBytes)
	if !c.enum("auth_mode", s.AuthMode,
		string(HTTPAuthModeBearer), string(HTTPAuthModeHMAC), string(HTTPAuthModeOAuth2)) {
		return
	}
	if HTTPAuthMode(s.AuthMode) == HTTPAuthModeOAuth2 {
		if s.PrincipalID != "" {
			c.add(CodeForbidden, "principal_id", "must be empty for oauth2; the client ID claim names the caller")
		}
		if s.CredentialBinding != "" {
			c.add(CodeForbidden, "credential_binding", "must be empty for oauth2")
		}
		if s.OAuth == nil {
			c.add(CodeRequired, "oauth", "is required for oauth2")
			return
		}
		for _, problem := range httpOAuthProblem(s.oauthPolicy()) {
			c.add(problem.code, "oauth."+problem.path, problem.message)
		}
		return
	}
	c.identity("principal_id", s.PrincipalID)
	c.binding("credential_binding", s.CredentialBinding)
	if s.OAuth != nil {
		c.add(CodeForbidden, "oauth", "applies only to auth_mode oauth2")
	}
}

// oauthPolicy fills the runtime defaults into the OAuth block.
func (s *HTTPSpec) oauthPolicy() HTTPOAuthPolicy {
	if s.OAuth == nil {
		return HTTPOAuthPolicy{}
	}
	policy := HTTPOAuthPolicy{
		IssuerURL:        s.OAuth.IssuerURL,
		Audience:         s.OAuth.Audience,
		TenantClaim:      s.OAuth.TenantClaim,
		RolesClaim:       s.OAuth.RolesClaim,
		ClientIDClaim:    s.OAuth.ClientIDClaim,
		SigningAlgs:      append([]string(nil), s.OAuth.SigningAlgs...),
		AllowedClientIDs: append([]string(nil), s.OAuth.AllowedClientIDs...),
	}
	if policy.TenantClaim == "" {
		policy.TenantClaim = DefaultHTTPOAuthTenantClaim
	}
	if policy.RolesClaim == "" {
		policy.RolesClaim = DefaultHTTPOAuthRolesClaim
	}
	if policy.ClientIDClaim == "" {
		policy.ClientIDClaim = DefaultHTTPOAuthClientIDClaim
	}
	if len(policy.SigningAlgs) == 0 {
		policy.SigningAlgs = []string{DefaultHTTPOAuthSigningAlg}
	}
	return policy
}

func (s *HTTPSpec) input(artifactID, revisionID string) HTTPSourceRevisionInput {
	input := HTTPSourceRevisionInput{
		ArtifactID:        artifactID,
		RevisionID:        revisionID,
		SourceID:          s.SourceID,
		Path:              s.Path,
		AuthMode:          HTTPAuthMode(s.AuthMode),
		PrincipalID:       s.PrincipalID,
		CredentialBinding: s.CredentialBinding,
		MaxBodyBytes:      int64Value(s.MaxBodyBytes),
	}
	if input.Path == "" {
		input.Path = DefaultHTTPSourcePath
	}
	if s.OAuth != nil {
		policy := s.oauthPolicy()
		input.OAuth = &policy
	}
	return input
}
