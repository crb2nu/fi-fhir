# EDI X12 Complexities Planning

This document details X12 healthcare transaction sets, loop structures, situational rules, and payer-specific variations for fi-fhir.

## X12 Transaction Set Overview

### Healthcare Transaction Sets (HIPAA)

| Code | Name | Direction | Implementation Guide |
|------|------|-----------|---------------------|
| **837P** | Professional Claim | Provider → Payer | 005010X222A1 |
| **837I** | Institutional Claim | Provider → Payer | 005010X223A3 |
| **837D** | Dental Claim | Provider → Payer | 005010X224A3 |
| **835** | Payment/Remittance | Payer → Provider | 005010X221A1 |
| **270** | Eligibility Inquiry | Provider → Payer | 005010X279A1 |
| **271** | Eligibility Response | Payer → Provider | 005010X279A1 |
| **276** | Claim Status Request | Provider → Payer | 005010X212 |
| **277** | Claim Status Response | Payer → Provider | 005010X212 |
| **278** | Prior Authorization | Bidirectional | 005010X217 |
| **834** | Enrollment | Sponsor ↔ Payer | 005010X220A1 |
| **820** | Premium Payment | Sponsor → Payer | 005010X218 |

### Version History

| Version | Year | Status |
|---------|------|--------|
| 4010/4010A1 | 2003-2012 | Deprecated |
| 5010 | 2012-present | Current HIPAA |
| 6020 | TBD | Future |

## X12 Envelope Structure

### Three-Level Hierarchy

```
ISA ... IEA    ← Interchange (outer envelope)
│
├─ GS ... GE   ← Functional Group (transaction type grouping)
│  │
│  ├─ ST ... SE   ← Transaction Set (individual claim/response)
│  │
│  └─ ST ... SE   ← Another transaction set
│
└─ GS ... GE   ← Another functional group
```

### ISA Segment (Interchange Header)

```
ISA*00*          *00*          *ZZ*SENDER         *ZZ*RECEIVER       *240115*0800*^*00501*000000001*0*P*:~
│   │            │            │  │              │  │               │      │    │ │     │         │ │ │
1   2            4            5  6              7  8               9     10   11 12   13        14 15 16

Key fields:
ISA05/06: Sender ID Qualifier and ID
ISA07/08: Receiver ID Qualifier and ID
ISA12: Interchange Control Version (00501 for 5010)
ISA13: Interchange Control Number (unique per interchange)
ISA15: Usage Indicator (P=Production, T=Test)
ISA16: Component Element Separator (:)
```

### GS Segment (Functional Group Header)

```
GS*HC*SENDERID*RECEIVERID*20240115*0800*1*X*005010X222A1~
│   │  │        │          │        │    │ │ │
1   2  3        4          5        6    7 8 9

GS01: Functional ID (HC=Healthcare Claim)
GS08: Version/Implementation Guide (005010X222A1)
```

## Loop Structures

### Understanding Loops

Loops are repeating groups of segments. Critical for claims:

```
2000A - Billing Provider Hierarchical Level
├── 2010AA - Billing Provider Name
├── 2010AB - Pay-to Address (situational)
│
2000B - Subscriber Hierarchical Level
├── 2010BA - Subscriber Name
├── 2010BB - Payer Name
│
2000C - Patient Hierarchical Level (if different from subscriber)
├── 2010CA - Patient Name
│
2300 - Claim Information (repeats per claim)
├── 2310A - Referring Provider
├── 2310B - Rendering Provider
├── 2320 - Other Subscriber (COB)
├── 2400 - Service Line (repeats per line item)
│   ├── 2420A - Rendering Provider (line level)
│   └── 2430 - Line Adjudication (in 835)
```

### Hierarchical Level (HL) Segment

```
HL*1**20*1~     ← HL Level 1, no parent, code 20 (Billing Provider), has children
HL*2*1*22*1~   ← HL Level 2, parent is 1, code 22 (Subscriber), has children
HL*3*2*23*0~   ← HL Level 3, parent is 2, code 23 (Patient), no children

HL Level Codes:
20 = Information Source (Billing Provider in 837)
21 = Information Receiver
22 = Subscriber
23 = Dependent (Patient)
```

## 837P (Professional Claim) Deep Dive

### Required Loops and Segments

```
Required:
- ISA/IEA (Interchange)
- GS/GE (Functional Group)
- ST/SE (Transaction Set)
- BHT (Beginning of Hierarchical Transaction)
- 1000A NM1 (Submitter)
- 1000B NM1 (Receiver)
- 2000A HL (Billing Provider Level)
- 2010AA NM1 (Billing Provider Name)
- 2000B HL (Subscriber Level)
- 2010BA NM1 (Subscriber Name)
- 2010BB NM1 (Payer Name)
- 2300 CLM (Claim)
- 2400 LX/SV1 (Service Line)
```

### CLM Segment Structure

```
CLM*CLAIM123*150.00***11:B:1*Y*A*Y*Y~
│   │         │       │       │ │ │ │ │
1   2         3       5       6 7 8 9 10

CLM01: Claim ID
CLM02: Total Claim Charge Amount
CLM05: Place of Service Code (11=Office, composite)
       Format: Facility:Frequency:Type
CLM06: Provider Signature Indicator
CLM07: Assignment Code
CLM08: Benefits Assignment Certification
CLM09: Release of Information Code
```

### SV1 Segment (Professional Service)

```
SV1*HC:99213:25*75.00*UN*1***1:2:3~
│   │  │     │  │     │  │   │
1   2  │     │  3     4  5   7
       │     └── Modifier
       └── HCPCS/CPT Code

SV101: Composite Medical Procedure (HC: indicates HCPCS)
SV102: Line Item Charge Amount
SV103: Unit Basis (UN=Unit)
SV104: Service Unit Count
SV107: Composite Diagnosis Code Pointer (references HI segment)
```

## 835 (Remittance) Deep Dive

### Loop Structure

```
1000A - Payer Identification
1000B - Payee Identification

2000 - Header Number (repeats per check/EFT)
├── 2100 - Claim Payment Information (repeats per claim)
│   ├── 2110 - Service Payment Information (repeats per line)
```

### CLP Segment (Claim Payment)

```
CLP*CLAIM123*1*150.00*100.00**12*PAYERID123*11*1~
│   │        │ │      │       │  │          │  │
1   2        3 4      5       6  7          8  9

CLP01: Claim ID (matches CLM01 from 837)
CLP02: Claim Status Code (1=Processed as Primary)
CLP03: Claim Charge Amount
CLP04: Claim Payment Amount
CLP06: Claim Filing Indicator Code (12=PPO)
CLP07: Payer Claim Control Number
CLP08: Facility Code
CLP09: Claim Frequency Code
```

### CAS Segment (Claim Adjustment)

```
CAS*CO*45*30.00*1~
│   │  │  │     │
1   2  3  4     5

CAS01: Claim Adjustment Group Code
  CO = Contractual Obligations
  PR = Patient Responsibility
  OA = Other Adjustments
  PI = Payer Initiated
  CR = Correction

CAS02: Adjustment Reason Code (CARC)
CAS03: Adjustment Amount
CAS04: Adjustment Quantity
```

## Situational Rules

### What "Situational" Means

```
Required (R):     Always send
Situational (S):  Send if condition met
Not Used (N):     Never send in this context
```

### Common Situational Scenarios

#### REF Segment Situational Use

```
REF*EI*123456789~  ← Required if billing provider has Tax ID
REF*SY*111223333~  ← Required if subscriber SSN needed for identification

Situational conditions in 837P:
- REF*G2 (Provider Commercial Number): Required if different from billing NPI
- REF*LU (Location Number): Required if claim-level location differs
```

#### NM1 Situational Loops

```
2310A - Referring Provider
  Required IF: Referring provider exists and different from rendering

2310B - Rendering Provider
  Required IF: Service was rendered by someone other than billing provider

2310C - Service Facility Location
  Required IF: Service location different from billing provider address
```

### Conditional Logic in fi-fhir

```go
type SituationalRule struct {
    Segment    string
    Condition  string // Expression like "claim.referring_provider != nil"
    Action     string // "required", "omit", "optional"
}

var rules837P = []SituationalRule{
    {
        Segment:   "2310A",
        Condition: "claim.referring_provider != nil && claim.referring_provider.npi != claim.rendering_provider.npi",
        Action:    "required",
    },
    {
        Segment:   "REF*G2",
        Condition: "provider.legacy_id != '' && provider.legacy_id != provider.npi",
        Action:    "required",
    },
}
```

## Payer-Specific Variations (Companion Guides)

### Why Companion Guides Exist

HIPAA mandates X12 format, but payers interpret/extend it:
- Different required fields
- Custom loop iterations
- Specific code values
- Extended validation rules

### Medicare-Specific Requirements

```yaml
# Medicare 837P variations
medicare:
  # Always requires NPI (no legacy numbers)
  provider_id:
    type: NPI
    legacy_allowed: false

  # Specific claim filing indicator
  claim_filing_indicator: "MA" # Medicare Part A or "MB" Medicare Part B

  # Requires specific type of bill
  facility_type_code_required: true

  # COB: Medicare always secondary payer info in specific format
  cob_loop_required_if_secondary: true
```

### Commercial Payer Variations

```yaml
# Blue Cross Blue Shield (varies by state)
bcbs_va:
  # Requires provider tax ID in specific REF segment
  tax_id_ref: "EI"

  # Wants subscriber ID without prefix
  subscriber_id_format: "strip_prefix"

  # Uses custom adjustment reason codes
  custom_carc_codes:
    - code: "B99"
      meaning: "BCBS-specific adjustment"

# United Healthcare
uhc:
  # Requires specific prior auth format
  prior_auth_ref: "G1"

  # Facility code mappings differ
  place_of_service_mappings:
    telehealth: "02"  # vs standard "11"
```

### Companion Guide Configuration

```yaml
# fi-fhir companion guide config
companion_guides:
  medicare_part_b:
    base_guide: "005010X222A1"
    overrides:
      - path: "2010AA.NM109"
        requirement: "required"
        validation: "npi_only"

      - path: "2000A.PRV"
        requirement: "required"
        note: "Provider taxonomy required"

    validations:
      - rule: "subscriber_id_format"
        pattern: "^[0-9A-Z]{11}$"  # MBI format

  bcbs_virginia:
    base_guide: "005010X222A1"
    overrides:
      - path: "2010AA.REF*EI"
        requirement: "required"

      - path: "2300.HI"
        max_diagnoses: 12  # vs standard 12

    custom_segments:
      - id: "NTE"
        position: "after:CLM"
        max_repeats: 5
```

## EDI Parsing Strategy

### Segment Parser

```go
type EDISegment struct {
    ID       string
    Elements []string
    Raw      string
}

func parseSegment(line string, elementSep byte) EDISegment {
    parts := strings.Split(line, string(elementSep))
    return EDISegment{
        ID:       parts[0],
        Elements: parts[1:],
        Raw:      line,
    }
}
```

### Loop State Machine

```go
type LoopState struct {
    CurrentLoop string
    LoopStack   []string
    Claims      []*Claim
    CurrentClaim *Claim
}

func (s *LoopState) ProcessSegment(seg EDISegment) {
    switch seg.ID {
    case "HL":
        hlCode := seg.Elements[2]
        switch hlCode {
        case "20":
            s.enterLoop("2000A")
        case "22":
            s.enterLoop("2000B")
        case "23":
            s.enterLoop("2000C")
        }

    case "CLM":
        s.CurrentClaim = &Claim{
            ID: seg.Elements[0],
        }
        s.enterLoop("2300")

    case "LX":
        // New service line
        s.enterLoop("2400")

    case "SE":
        // End of transaction
        s.finalizeTransaction()
    }
}
```

### Hierarchical Navigation

```go
type HierarchicalTree struct {
    Levels map[string]*HLNode
}

type HLNode struct {
    ID       string
    ParentID string
    Code     string // 20, 22, 23
    Segments []EDISegment
    Children []*HLNode
}

func buildHierarchy(segments []EDISegment) *HierarchicalTree {
    tree := &HierarchicalTree{
        Levels: make(map[string]*HLNode),
    }

    for _, seg := range segments {
        if seg.ID == "HL" {
            node := &HLNode{
                ID:       seg.Elements[0],
                ParentID: seg.Elements[1],
                Code:     seg.Elements[2],
            }
            tree.Levels[node.ID] = node

            if node.ParentID != "" {
                parent := tree.Levels[node.ParentID]
                if parent != nil {
                    parent.Children = append(parent.Children, node)
                }
            }
        }
    }

    return tree
}
```

## Semantic Mapping to fi-fhir Events

### 837 → Claim Event

```go
func map837ToClaim(tx *Transaction837) *events.ClaimEvent {
    claim := &events.ClaimEvent{
        EventMeta: events.NewEventMeta(
            events.EventClaimSubmitted,
            "edi_translator",
            events.FormatEDI837,
        ),
    }

    // Map billing provider
    loop2010AA := tx.GetLoop("2010AA")
    claim.BillingProvider = mapNM1ToProvider(loop2010AA.GetSegment("NM1"))

    // Map subscriber
    loop2010BA := tx.GetLoop("2010BA")
    claim.Subscriber = mapNM1ToPatient(loop2010BA.GetSegment("NM1"))

    // Map claim details
    clm := tx.GetSegment("CLM")
    claim.ClaimID = clm.Elements[0]
    claim.TotalAmount = parseDecimal(clm.Elements[1])

    // Map service lines
    for _, loop2400 := range tx.GetLoops("2400") {
        line := mapServiceLine(loop2400)
        claim.ServiceLines = append(claim.ServiceLines, line)
    }

    return claim
}
```

### 835 → ClaimAdjudicated Event

```go
func map835ToAdjudication(tx *Transaction835) []*events.ClaimAdjudicatedEvent {
    var events []*events.ClaimAdjudicatedEvent

    for _, clp := range tx.GetSegments("CLP") {
        event := &events.ClaimAdjudicatedEvent{
            ClaimID:        clp.Elements[0],
            Status:         mapClaimStatus(clp.Elements[1]),
            ChargedAmount:  parseDecimal(clp.Elements[2]),
            PaidAmount:     parseDecimal(clp.Elements[3]),
        }

        // Get adjustments
        for _, cas := range tx.GetRelatedSegments(clp, "CAS") {
            adj := mapAdjustment(cas)
            event.Adjustments = append(event.Adjustments, adj)
        }

        events = append(events, event)
    }

    return events
}
```

## Implementation Plan

### Phase 1: Core X12 Parsing
- [ ] Envelope parsing (ISA/GS/ST)
- [ ] Segment tokenization
- [ ] Element/component separation
- [ ] Basic validation (segment terminator, element separator)

### Phase 2: Loop Recognition
- [ ] HL-based hierarchy building
- [ ] Loop start/end detection
- [ ] State machine for 837P
- [ ] State machine for 835

### Phase 3: Semantic Extraction
- [ ] 837P → ClaimSubmitted event
- [ ] 835 → ClaimAdjudicated event
- [ ] 270/271 → EligibilityCheck event
- [ ] Error handling for malformed EDI

### Phase 4: Companion Guide Framework
- [ ] Configuration schema for payer rules
- [ ] Validation engine
- [ ] Medicare guide
- [ ] 2-3 major commercial payer guides

## Testing Strategy

### Sample Files Needed

```
testdata/edi/
├── 837p_minimal.edi       # Single claim, single line
├── 837p_multiple.edi      # Multiple claims per file
├── 837p_cob.edi           # Coordination of benefits
├── 835_single.edi         # Single remittance
├── 835_multiple.edi       # Multiple claims in ERA
├── 270_inquiry.edi        # Eligibility check
├── 271_response.edi       # Eligibility response
└── invalid/
    ├── bad_envelope.edi
    ├── missing_hl.edi
    └── malformed_clm.edi
```

### Validation Tests

| Test Case | Input | Expected |
|-----------|-------|----------|
| Valid 837P | Minimal claim | Parses to ClaimEvent |
| Missing NPI | No NM109 | Validation error |
| Invalid CLP status | CLP02 = "X" | Unknown status warning |
| COB claim | Secondary payer present | Both payers in event |

## References

- [X12 837P Implementation Guide (5010)](https://x12.org/products/5010-702)
- [CMS EDI Requirements](https://www.cms.gov/medicare/billing/electronicbillingeditrans)
- [Washington Publishing Company Guides](https://www.wpc-edi.com/)
- [Stedi EDI Reference](https://www.stedi.com/edi)
