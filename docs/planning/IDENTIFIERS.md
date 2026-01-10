# Healthcare Identifier Systems Planning

This document details patient and provider identification systems, normalization strategies, and matching logic for fi-fhir.

## Quick Reference

| Validator | Status | Checksum | Implementation |
|-----------|--------|----------|----------------|
| **NPI** | ✅ | Luhn (80840 prefix) | `pkg/validate/identifiers.go:NPIValidator` |
| **MBI** | ✅ | Format rules | `pkg/validate/identifiers.go:MBIValidator` |
| **SSN** | ✅ | Area/group rules | `pkg/validate/identifiers.go:SSNValidator` |
| **DEA** | ✅ | Weighted checksum | `pkg/validate/identifiers.go:DEAValidator` |

| Normalizer | Purpose |
|------------|---------|
| **SSN** | Strip dashes, reject invalid patterns (000000000) |
| **Phone** | Strip country code, normalize to 10 digits |

## Patient Identifiers

### Identifier Types Reference

| ID Type | OID | Format | Assigner | Persistence |
|---------|-----|--------|----------|-------------|
| **MRN** | Facility-specific | Varies | Hospital/Clinic | Per facility |
| **Enterprise MRN** | Health system OID | Varies | Health system | Across facilities |
| **SSN** | 2.16.840.1.113883.4.1 | XXX-XX-XXXX | SSA | Lifetime |
| **MBI** | 2.16.840.1.113883.4.927 | XAXX-XXX-XXXX | CMS | Medicare enrollment |
| **Medicaid ID** | State-specific OID | Varies by state | State Medicaid | Enrollment period |
| **Insurance Member ID** | Payer OID | Varies | Payer | Policy period |
| **Driver's License** | 2.16.840.1.113883.4.3.[state] | Varies | State DMV | Until renewal |

### MBI (Medicare Beneficiary Identifier)

Replaced HICN (Health Insurance Claim Number) in 2020.

```
Format: XAXX-XXX-XXXX (11 characters)

Position  Allowed Characters
1         1-9 (no 0)
2         A-Z (no S,L,O,I,B,Z)
3         Alphanumeric (no S,L,O,I,B,Z)
4         0-9
5         A-Z (no S,L,O,I,B,Z)
6-7       Alphanumeric (no S,L,O,I,B,Z)
8         0-9
9-11      Alphanumeric (no S,L,O,I,B,Z)

Example: 1EG4-TE5-MK72
```

### State Medicaid ID Formats

| State | Format | Example | Notes |
|-------|--------|---------|-------|
| CA | 9 digits | 123456789 | Medi-Cal |
| NY | 8 alphanumeric | AB123456 | |
| TX | 9 digits | 012345678 | |
| FL | 10 digits | 1234567890 | |
| VA | 12 digits | 123456789012 | |

### HL7v2 PID-3 Structure (Patient Identifier List)

```
PID-3: Patient Identifier List (CX data type, repeating)

CX Components:
1. ID Number (ST)           - The actual identifier value
2. Check Digit (ST)         - Optional validation digit
3. Check Digit Scheme (ID)  - Algorithm used (M10, M11, etc.)
4. Assigning Authority (HD) - Who issued the ID
5. Identifier Type Code (ID)- Type (MR, SS, AN, etc.)
6. Assigning Facility (HD)  - Facility issuing
7-10. Various optional fields

Example:
123456789^^^HOSPITAL_A^MRN~111-22-3333^^^SSA^SS~BC1234567^^^BCBS^PI
    │         │           │        │      │       │        │    │
    ID     Assigner     Type      ID    Assigner Type     ID   Type
```

### Identifier Type Codes (HL7 Table 0203)

| Code | Name | Description |
|------|------|-------------|
| MR | Medical Record Number | Facility MRN |
| PI | Patient Internal ID | Organization-specific |
| SS | Social Security Number | US SSN |
| AN | Account Number | Billing account |
| PT | Patient External ID | External system ID |
| MB | Member Number | Insurance member ID |
| MA | Medicaid Number | State Medicaid |
| MC | Medicare Number | Medicare (MBI/HICN) |
| DL | Driver's License | State-issued |
| PPN | Passport Number | International ID |

## Provider Identifiers

### NPI (National Provider Identifier)

```
Format: 10 digits, Luhn check digit

Type 1: Individual providers (physicians, nurses, etc.)
Type 2: Organizations (hospitals, groups, etc.)

Validation:
1. Must be exactly 10 digits
2. Luhn algorithm checksum (prefix "80840")
3. First digit cannot be 0

Example: 1234567893
```

```go
// NPI validation
func ValidateNPI(npi string) bool {
    if len(npi) != 10 || !isNumeric(npi) {
        return false
    }

    // Luhn check with 80840 prefix
    prefixed := "80840" + npi
    return luhnCheck(prefixed)
}

func luhnCheck(number string) bool {
    sum := 0
    alternate := false
    for i := len(number) - 1; i >= 0; i-- {
        n := int(number[i] - '0')
        if alternate {
            n *= 2
            if n > 9 {
                n -= 9
            }
        }
        sum += n
        alternate = !alternate
    }
    return sum%10 == 0
}
```

### DEA Number

```
Format: 2 letters + 6 digits + 1 check digit

First Letter: Registrant type
  A, B, F, G = Manufacturers, distributors
  M = Mid-level practitioners
  P, R = Researchers

Second Letter: First letter of last name (usually)

Check Digit Formula:
1. Add 1st, 3rd, 5th digits
2. Add 2nd, 4th, 6th digits, multiply by 2
3. Sum of steps 1 and 2, last digit = check digit

Example: AB1234563
  Step 1: 1 + 3 + 5 = 9
  Step 2: (2 + 4 + 6) × 2 = 24
  Sum: 9 + 24 = 33, check digit = 3 ✓
```

### Provider ID Cross-Reference

| System | Example | Use Case |
|--------|---------|----------|
| NPI | 1234567893 | Universal provider ID |
| DEA | AB1234563 | Controlled substance prescribing |
| State License | VA-12345 | State-specific practice |
| Medicare PTAN | I12345 | Medicare billing |
| Medicaid ID | (varies) | Medicaid billing |
| Hospital ID | DR001 | Internal systems |
| UPIN (legacy) | A12345 | Pre-NPI Medicare |

## Patient Matching Strategy

### The Challenge

No universal patient ID means probabilistic matching based on demographics:

```
Input: New patient record
- Name: John W Smith
- DOB: 1965-03-15
- SSN: (declined)
- Phone: 555-123-4567
- Address: 123 Main St, Anytown VA

Question: Is this the same "John Smith" we already have?
```

### Matching Algorithm Options

#### 1. Deterministic Matching
Exact match on one or more fields:
```go
type DeterministicMatch struct {
    Fields   []string // e.g., ["ssn"] or ["mrn", "facility"]
    Required bool
}

// Match if SSN matches exactly
if record1.SSN == record2.SSN && record1.SSN != "" {
    return MatchConfirmed
}
```

#### 2. Probabilistic Matching
Weighted scoring across multiple fields:

| Field | Weight | Match Type |
|-------|--------|------------|
| SSN | 0.95 | Exact |
| MRN + Facility | 0.90 | Exact |
| DOB | 0.80 | Exact |
| Last Name | 0.60 | Soundex/phonetic |
| First Name | 0.50 | Soundex/phonetic |
| Address | 0.40 | Normalized comparison |
| Phone | 0.70 | Last 7 digits |
| Gender | 0.30 | Exact |

```go
type MatchScore struct {
    Score      float64 // 0.0 - 1.0
    Confidence string  // "high", "medium", "low", "no_match"
    Details    map[string]float64 // Per-field scores
}

func ScoreMatch(a, b *Patient) MatchScore {
    score := 0.0
    maxScore := 0.0

    // SSN match
    if a.SSN != "" && b.SSN != "" {
        maxScore += 0.95
        if a.SSN == b.SSN {
            score += 0.95
        }
    }

    // DOB match
    maxScore += 0.80
    if !a.DOB.IsZero() && a.DOB.Equal(b.DOB) {
        score += 0.80
    }

    // Name matching (phonetic)
    maxScore += 0.60
    if soundex(a.LastName) == soundex(b.LastName) {
        score += 0.60
    }

    // ... more fields

    normalized := score / maxScore
    return MatchScore{
        Score: normalized,
        Confidence: classifyConfidence(normalized),
    }
}
```

#### 3. Machine Learning Matching
Train model on confirmed match/no-match pairs:
- Input features: field similarity scores
- Output: match probability
- Requires labeled training data

### Match Thresholds

```yaml
matching:
  thresholds:
    confirmed: 0.95  # Auto-link records
    review: 0.70     # Human review needed
    no_match: 0.40   # Definitely different

  rules:
    # SSN alone confirms match
    - condition: ssn_exact_match
      action: confirmed

    # MRN + DOB confirms
    - condition: mrn_exact_match AND dob_exact_match
      action: confirmed

    # Name + DOB + phone suggests review
    - condition: name_phonetic_match AND dob_exact_match AND phone_partial
      action: review
```

## Identifier Normalization

### SSN Normalization
```go
func NormalizeSSN(input string) string {
    // Remove non-digits
    digits := regexp.MustCompile(`\d`).FindAllString(input, -1)
    ssn := strings.Join(digits, "")

    // Must be 9 digits
    if len(ssn) != 9 {
        return ""
    }

    // Invalid patterns (all same digit, sequential)
    if ssn == "000000000" || ssn == "123456789" {
        return ""
    }

    return ssn
}
```

### Phone Normalization
```go
func NormalizePhone(input string) string {
    digits := regexp.MustCompile(`\d`).FindAllString(input, -1)
    phone := strings.Join(digits, "")

    // Remove country code if present
    if len(phone) == 11 && phone[0] == '1' {
        phone = phone[1:]
    }

    if len(phone) != 10 {
        return ""
    }

    return phone
}
```

### Name Normalization
```go
func NormalizeName(input string) string {
    // Uppercase
    name := strings.ToUpper(input)

    // Remove prefixes/suffixes
    prefixes := []string{"MR", "MRS", "MS", "DR", "MISS"}
    suffixes := []string{"JR", "SR", "II", "III", "IV", "MD", "PHD"}

    for _, p := range prefixes {
        name = strings.TrimPrefix(name, p+" ")
        name = strings.TrimPrefix(name, p+".")
    }

    // Remove punctuation
    name = regexp.MustCompile(`[^A-Z\s]`).ReplaceAllString(name, "")

    // Collapse whitespace
    name = strings.Join(strings.Fields(name), " ")

    return name
}
```

## fi-fhir Canonical Identifier Model

```go
// Identifier represents any healthcare identifier
type Identifier struct {
    // The identifier value (normalized)
    Value string `json:"value"`

    // Code system URI or OID
    System string `json:"system"`

    // Type code (from HL7 Table 0203)
    Type string `json:"type"`

    // Who assigned this identifier
    Assigner string `json:"assigner,omitempty"`

    // Original value before normalization
    OriginalValue string `json:"original_value,omitempty"`

    // Validity period
    Period *Period `json:"period,omitempty"`

    // Use (usual, official, temp, secondary, old)
    Use string `json:"use,omitempty"`
}

// IdentifierSet manages multiple identifiers for an entity
type IdentifierSet struct {
    Identifiers []Identifier `json:"identifiers"`

    // Primary identifier (usually MRN from source)
    Primary *Identifier `json:"primary,omitempty"`
}

func (s *IdentifierSet) GetByType(idType string) *Identifier {
    for _, id := range s.Identifiers {
        if id.Type == idType {
            return &id
        }
    }
    return nil
}

func (s *IdentifierSet) GetBySystem(system string) []Identifier {
    var result []Identifier
    for _, id := range s.Identifiers {
        if id.System == system {
            result = append(result, id)
        }
    }
    return result
}
```

## Implementation Plan

### Phase 1: Core Identifier Parsing ✅
- [x] Parse HL7v2 PID-3 (repeating CX) - see `internal/parser/hl7v2/parser.go`
- [x] Parse HL7v2 XCN (provider names with IDs)
- [x] Basic normalization (SSN, phone) - see `pkg/validate/identifiers.go`

### Phase 2: Validation ✅
- [x] NPI Luhn validation - see `pkg/validate/identifiers.go:NPIValidator`
- [x] DEA checksum validation - see `pkg/validate/identifiers.go:DEAValidator`
- [x] MBI format validation - see `pkg/validate/identifiers.go:MBIValidator`
- [x] SSN reasonableness checks - see `pkg/validate/identifiers.go:SSNValidator`
- [x] Phone/SSN normalizers - see `pkg/validate/identifiers.go`

### Phase 3: Matching Engine 🔲
- [ ] Deterministic matching rules
- [ ] Probabilistic scoring (Jaro-Winkler, Soundex)
- [ ] Configurable thresholds

### Phase 4: MPI Interface 🔲
- [ ] Abstract MPI interface
- [ ] In-memory implementation for testing
- [ ] External MPI integration (future)

> **Note**: Matching engine and MPI interface are planned but not yet implemented. See backlog in [README.md](README.md).

## Testing Data

```go
var testPatients = []Patient{
    {
        MRN: "123456",
        Identifiers: []Identifier{
            {Value: "123456", Type: "MR", System: "urn:oid:1.2.3.4"},
            {Value: "111223333", Type: "SS", System: "2.16.840.1.113883.4.1"},
        },
        FamilyName: "SMITH",
        GivenName: "JOHN",
        DOB: time.Date(1965, 3, 15, 0, 0, 0, 0, time.UTC),
    },
    // Similar patient - should match?
    {
        MRN: "789012",
        Identifiers: []Identifier{
            {Value: "789012", Type: "MR", System: "urn:oid:5.6.7.8"},
            {Value: "111223333", Type: "SS", System: "2.16.840.1.113883.4.1"},
        },
        FamilyName: "SMITH",
        GivenName: "JOHN W",
        DOB: time.Date(1965, 3, 15, 0, 0, 0, 0, time.UTC),
    },
}
```

## See Also

- [SOURCE-PROFILES.md](SOURCE-PROFILES.md) - Identifier validation config per source profile
- [HL7V2-QUIRKS.md](HL7V2-QUIRKS.md) - PID-3 parsing and CX data type variations
- [FHIR-PROFILES.md](FHIR-PROFILES.md) - FHIR Identifier type and system URIs
- [EDI-COMPLEXITIES.md](EDI-COMPLEXITIES.md) - NPI validation in EDI NM1 segments

## References

- [NPI Registry](https://npiregistry.cms.hhs.gov/)
- [MBI Format](https://www.cms.gov/medicare/new-medicare-card/understanding-the-mbi.pdf)
- [HL7 v2.5 Table 0203](https://hl7-definition.caristix.com/v2/HL7v2.5/Tables/0203)
- [DEA Registration](https://www.deadiversion.usdoj.gov/)
