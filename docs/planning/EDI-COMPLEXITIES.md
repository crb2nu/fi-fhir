# EDI X12 Complexities Planning

This document details X12 healthcare transaction sets, loop structures, situational rules, and payer-specific variations for fi-fhir.

## Quick Reference

| Transaction | Status | Events | Implementation |
|-------------|--------|--------|----------------|
| **837P** | ✅ | `ClaimSubmittedEvent` | `internal/parser/edi/mapper.go:Map837ToEvents()` |
| **835** | ✅ | `ClaimAdjudicatedEvent` | `internal/parser/edi/mapper.go:Map835ToEvents()` |
| **270** | ✅ | `EligibilityInquiryEvent` | `internal/parser/edi/mapper.go:Map270ToEvents()` |
| **271** | ✅ | `EligibilityResponseEvent` | `internal/parser/edi/mapper.go:Map271ToEvents()` |
| **276/277** | 🔲 | `ClaimStatusEvent` | Planned |

| Component | Implementation |
|-----------|----------------|
| **Envelope parsing** | `internal/parser/edi/parser.go:parseInterchange()` |
| **Loop detection** | `internal/parser/edi/loops.go:Parse837Loops()` |
| **HL hierarchy** | `internal/parser/edi/loops.go:BuildHLTree()` |

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

## 270 (Eligibility Inquiry) Deep Dive

### Loop Structure

```
1000A - Information Source Name (Payer)
1000B - Information Receiver Name (Provider/Clearinghouse)

2000A - Information Source Level (HL Code 20)
├── 2100A - Information Source Name (Payer details)

2000B - Information Receiver Level (HL Code 21)
├── 2100B - Information Receiver Name (Provider details)

2000C - Subscriber Level (HL Code 22)
├── 2100C - Subscriber Name
├── TRN - Trace Number (for correlation)
├── DTP - Date/Time Reference (service date)
├── EQ - Eligibility/Benefit Inquiry (repeats)

2000D - Dependent Level (HL Code 23) - Optional
├── 2100D - Dependent Name
├── TRN - Trace Number
├── EQ - Eligibility/Benefit Inquiry
```

### Key Segments

#### BHT Segment (Beginning of Hierarchical Transaction)

```
BHT*0022*13*ABC123*20240115*0900~
│   │    │  │      │        │
1   2    3  4      5        6

BHT01: Hierarchical Structure Code (0022 for 270/271)
BHT02: Transaction Purpose Code (13=Request)
BHT03: Reference Identification (Originator ID)
BHT04: Transaction Set Creation Date
BHT05: Transaction Set Creation Time
```

#### TRN Segment (Trace Number)

```
TRN*1*TRACE123456*9RECEIVER~
│   │ │           │
1   2 3           4

TRN01: Trace Type Code (1=Current Transaction)
TRN02: Reference Identification (your trace number)
TRN03: Originating Company Identifier
```

#### EQ Segment (Eligibility/Benefit Inquiry)

```
EQ*30~          ← Service type code only
EQ*30**FAM~     ← With coverage level

EQ01: Service Type Code
  30 = Health Benefit Plan Coverage
  33 = Chiropractic
  47 = Hospital
  48 = Hospital - Inpatient
  50 = Hospital - Outpatient
  86 = Emergency Services
  98 = Professional (Physician) Visit - Office
  AL = Vision (Optometry)
  MH = Mental Health

EQ02: Composite Medical Procedure (optional CPT/HCPCS)
EQ03: Coverage Level Code (optional)
  IND = Individual
  FAM = Family
  EMP = Employee Only
  ESP = Employee and Spouse
```

### Semantic Mapping

```go
// 270 maps to EligibilityInquiryEvent
// Key extractions:
// - Information Source (payer) from 2100A NM1
// - Information Receiver (provider) from 2100B NM1
// - Subscriber from 2100C NM1 + DMG
// - Dependent from 2100D NM1 + DMG (if present)
// - Service types from EQ segments
// - Trace number from TRN segment
```

## 271 (Eligibility Response) Deep Dive

### Loop Structure

```
1000A - Information Source Name (Payer)
1000B - Information Receiver Name (Provider/Clearinghouse)

2000A - Information Source Level (HL Code 20)
├── 2100A - Information Source Name
├── AAA - Request Validation (payer-level errors)

2000B - Information Receiver Level (HL Code 21)
├── 2100B - Information Receiver Name

2000C - Subscriber Level (HL Code 22)
├── 2100C - Subscriber Name
├── TRN - Trace Number (echoed from 270)
├── AAA - Request Validation (subscriber-level errors)
├── DTP - Eligibility/Benefit Date (plan dates)
├── EB - Eligibility/Benefit Information (repeats)
│   └── EB segment can reference diagnosis, procedure codes

2000D - Dependent Level (HL Code 23) - Optional
├── 2100D - Dependent Name
├── AAA - Request Validation
├── EB - Eligibility/Benefit Information
```

### EB Segment (Eligibility/Benefit Information)

```
EB*1*IND*30*HM*GOLD PLAN~
│  │ │   │  │  │
1  2 3   4  5  6

EB*C*IND*30**25.00~        ← $25 copay
EB*G*IND*30**500.00*****23~  ← $500 deductible, calendar year

EB01: Eligibility/Benefit Information Code
  1 = Active Coverage
  2 = Active - Full Risk Capitation
  3 = Active - Services Capitated
  4 = Active - Services Capitated to Primary Care
  5 = Active - Pending Investigation
  6 = Inactive
  7 = Inactive - Pending Eligibility Update
  8 = Inactive - Pending Investigation
  A = Co-Insurance
  B = Co-Payment
  C = Deductible
  D = Benefit Description
  E = Exclusions
  F = Limitations
  G = Out of Pocket (Stop Loss)
  I = Non-Covered
  J = Cost Containment
  K = Reserve
  L = Primary Care Provider
  MC = Medicare Coverage Type

EB02: Coverage Level Code (IND, FAM, etc.)

EB03: Service Type Code (same as EQ01)

EB04: Insurance Type Code
  HM = HMO
  PPO = Preferred Provider Organization
  POS = Point of Service
  EPO = Exclusive Provider Organization
  IND = Indemnity
  MC = Medicaid
  MA = Medicare Part A
  MB = Medicare Part B

EB05: Plan Coverage Description (free text)

EB06: Time Period Qualifier
  22 = Service Year
  23 = Calendar Year
  24 = Year to Date
  25 = Contract
  26 = Episode
  27 = Visit
  29 = Remaining
  32 = Lifetime
  33 = Lifetime Remaining

EB07: Monetary Amount (deductible, copay, out-of-pocket)

EB08: Percentage (coinsurance)
```

### AAA Segment (Request Validation)

```
AAA*N**75*N~
│   │  │  │
1   2  3  4

AAA01: Valid Request Indicator (Y/N)
AAA02: Agency Qualifier Code (unused)
AAA03: Reject Reason Code
  04 = Authorized Quantity Exceeded
  15 = Required Application Data Missing
  33 = Input Errors
  41 = Authorization/Access Restrictions
  42 = Unable to Respond at Current Time
  43 = Invalid/Missing Provider Identification
  44 = Invalid/Missing Provider Name
  45 = Invalid/Missing Provider Specialty
  46 = Invalid/Missing Provider Phone Number
  47 = Invalid/Missing Provider State
  48 = Invalid/Missing Referring Provider ID
  50 = Provider Not on File
  51 = Provider Not Primary Care Physician
  52 = Provider Ineligible for Inquiries
  53 = Inquired Benefit Inconsistent with Provider Type
  54 = Inappropriate Product/Service ID Qualifier
  55 = Inappropriate Product/Service ID
  56 = Inappropriate Date
  57 = Invalid/Missing Dates of Service
  58 = Invalid/Missing Date of Birth
  60 = Date of Birth Follows Dates of Service
  61 = Date of Death Precedes Dates of Service
  62 = Date of Service Not Within Allowable Inquiry Period
  63 = Date of Service in Future
  64 = Invalid/Missing Patient ID
  65 = Invalid/Missing Patient Name
  66 = Invalid/Missing Patient Gender Code
  67 = Patient Not Found
  68 = Duplicate Patient ID Number
  69 = Inconsistent with Patient's Age
  70 = Inconsistent with Patient's Gender
  71 = Patient Birth Date Does Not Match
  72 = Invalid/Missing Subscriber ID
  73 = Invalid/Missing Subscriber Name
  74 = Invalid/Missing Subscriber Gender
  75 = Subscriber Not Found
  76 = Duplicate Subscriber ID Number
  77 = Subscriber Not in Group
  79 = Invalid Participant ID
  80 = No Response Received
  97 = Invalid/Missing Subscriber ID
  T4 = Payer Name or ID Missing

AAA04: Follow-up Action Code
  N = Not Applicable
  C = Please Correct and Resubmit
  P = Please Resubmit Original Transaction
  R = Resubmission Not Allowed
  S = Do Not Resubmit
  W = Please Wait 30 Days and Resubmit
  X = Please Wait 10 Days and Resubmit
```

### Semantic Mapping

```go
// 271 maps to EligibilityResponseEvent
// Key extractions:
// - Status: derived from EB01 codes (1-8 = active/inactive variants)
// - Benefits: from EB segments with monetary/percentage data
// - Errors: from AAA segments when validation fails
// - Plan dates: from DTP segments (348=Plan Begin, 349=Plan End)
// - Coverage details: EB05 description, EB04 insurance type
```

### Response Scenarios

#### Active Coverage (Happy Path)

```
EB*1*IND*30*HM*GOLD PLAN~                    ← Active, HMO, plan name
EB*C*IND*30**25.00****27~                    ← $25 copay per visit
EB*A*IND*30**20~                             ← 20% coinsurance
EB*G*IND*30**2500.00*****23~                 ← $2,500 annual out-of-pocket
DTP*348*D8*20240101~                         ← Plan begins Jan 1, 2024
DTP*349*D8*20241231~                         ← Plan ends Dec 31, 2024
```

#### Inactive Coverage

```
EB*6*IND*30~                                 ← Inactive
AAA*N**75*N~                                 ← Subscriber not found
```

#### Validation Error (No Coverage Inquiry)

```
AAA*N**67*N~                                 ← Patient not found
AAA*N**72*C~                                 ← Invalid subscriber ID, please correct
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

### Phase 1: Core X12 Parsing ✅
- [x] Envelope parsing (ISA/GS/ST) - see `parser.go:parseInterchange()`
- [x] Segment tokenization - see `parser.go:tokenizeSegments()`
- [x] Element/component separation with delimiter detection
- [x] Basic validation (segment terminator, element separator)
- [x] ParseWarning system for non-fatal issues

### Phase 2: Loop Recognition ✅
- [x] HL-based hierarchy building - see `loops.go:BuildHLTree()`
- [x] Loop start/end detection - see `loops.go:AssignSegmentsToHL()`
- [x] State machine for 837P - see `loops.go:Parse837Loops()`
- [x] State machine for 835 - see `loops.go:Parse835Loops()`
- [x] Full loop structure types (Loop2000A/B/C, Loop2300, Loop2400, etc.)

### Phase 3: Semantic Extraction
- [x] 837P → ClaimSubmittedEvent - see `mapper.go:Map837ToEvents()`
- [x] 835 → ClaimAdjudicatedEvent - see `mapper.go:Map835ToEvents()`
- [x] 270 → EligibilityInquiryEvent - see `mapper.go:Map270ToEvents()`
- [x] 271 → EligibilityResponseEvent - see `mapper.go:Map271ToEvents()`
- [ ] 276/277 → ClaimStatus event
- [x] Basic error handling with ParseError type

### Phase 4: Companion Guide Framework
- [ ] Configuration schema for payer rules
- [ ] Validation engine
- [ ] Medicare guide
- [ ] 2-3 major commercial payer guides

## Testing Strategy

### Sample Files

```
testdata/edi/
├── 837p_minimal.edi       # ✅ Single claim, single line
├── 837p_multiple.edi      # 🔲 Multiple claims per file
├── 837p_cob.edi           # 🔲 Coordination of benefits
├── 835_single.edi         # 🔲 Single remittance (inline in tests)
├── 835_multiple.edi       # 🔲 Multiple claims in ERA
├── 270_inquiry.edi        # ✅ Eligibility inquiry
├── 271_response.edi       # ✅ Eligibility response (active coverage)
├── 271_rejected.edi       # ✅ Eligibility rejected (errors)
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

## See Also

- [WORKFLOW-DSL.md](WORKFLOW-DSL.md) - Route EDI events to FHIR servers or webhooks
- [FHIR-PROFILES.md](FHIR-PROFILES.md) - FHIR R4 Claim and ExplanationOfBenefit resources
- [IDENTIFIERS.md](IDENTIFIERS.md) - NPI validation for provider identifiers in EDI

## References

- [X12 837P Implementation Guide (5010)](https://x12.org/products/5010-702)
- [CMS EDI Requirements](https://www.cms.gov/medicare/billing/electronicbillingeditrans)
- [Washington Publishing Company Guides](https://www.wpc-edi.com/)
- [Stedi EDI Reference](https://www.stedi.com/edi)
