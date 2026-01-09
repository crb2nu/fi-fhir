# HL7 v2.x Quirks & Variations Planning

This document details version differences, Z-segment handling, vendor variations, and parsing edge cases for fi-fhir.

## Version Compatibility Matrix

### Structural Differences

| Feature | v2.3 | v2.3.1 | v2.4 | v2.5 | v2.5.1 | v2.6+ |
|---------|------|--------|------|------|--------|-------|
| MSH-21 (Profile) | No | No | No | Yes | Yes | Yes |
| SFT segment | No | No | No | Yes | Yes | Yes |
| ERR segment | Basic | Basic | Basic | Enhanced | Enhanced | Enhanced |
| TQ1 (Timing) | No | No | No | Yes | Yes | Yes |
| BPO (Blood Product) | No | No | No | Yes | Yes | Yes |
| Escape sequences | Basic | Basic | Extended | Extended | Extended | Extended |

### Data Type Evolution

#### XCN (Extended Composite ID + Name)

```
v2.3:  9 components
v2.4:  14 components
v2.5+: 23 components

Components (v2.5+):
1. ID Number
2. Family Name
3. Given Name
4. Second/Middle Name
5. Suffix
6. Prefix
7. Degree
8. Source Table
9. Assigning Authority
10. Name Type Code
11. Identifier Check Digit
12. Check Digit Scheme
13. Identifier Type Code
14. Assigning Facility
15-23. Extended fields
```

#### XPN (Extended Person Name)

```
v2.3:  8 components
v2.5+: 14 components

Key differences:
- v2.5 added Professional Suffix (component 14)
- v2.5 added Effective/Expiration dates
```

#### CX (Extended Composite ID)

```
v2.3: 6 components
v2.5: 10 components

Components added in v2.5:
7. Effective Date
8. Expiration Date
9. Assigning Jurisdiction
10. Assigning Agency
```

## Message Encoding Variations

### Delimiter Handling

```
Standard MSH:
MSH|^~\&|...

Components:
MSH-1: Field separator (|)
MSH-2: Encoding characters (^~\&)
  Position 1: Component separator (^)
  Position 2: Repetition separator (~)
  Position 3: Escape character (\)
  Position 4: Subcomponent separator (&)

Non-standard examples seen in the wild:
MSH|^~\$|...  ($ instead of &)
MSH|!~\&|...  (! instead of ^)
```

### Escape Sequences

| Sequence | Meaning | Character |
|----------|---------|-----------|
| \F\ | Field separator | \| |
| \S\ | Component separator | ^ |
| \T\ | Subcomponent separator | & |
| \R\ | Repetition separator | ~ |
| \E\ | Escape character | \ |
| \Xhh\ | Hex character | (varies) |
| \.br\ | Line break | \n |
| \H\ | Start highlight | |
| \N\ | Normal (end highlight) | |

### Line Ending Variations

```go
// Normalize line endings
func normalizeLineEndings(raw string) string {
    // Spec says \r only, but we see all variations
    raw = strings.ReplaceAll(raw, "\r\n", "\r")
    raw = strings.ReplaceAll(raw, "\n", "\r")
    return raw
}
```

### Character Encoding Issues

| Encoding | Common Sources | Issues |
|----------|----------------|--------|
| ASCII | Legacy systems | Limited character set |
| ISO-8859-1 | European systems | Latin-1 special chars |
| UTF-8 | Modern systems | Multi-byte characters |
| Windows-1252 | Windows apps | Smart quotes, em-dashes |

```go
// Character encoding detection/handling
func detectEncoding(raw []byte) string {
    // Check for UTF-8 BOM
    if len(raw) >= 3 && raw[0] == 0xEF && raw[1] == 0xBB && raw[2] == 0xBF {
        return "UTF-8"
    }

    // Check MSH-18 (Character Set)
    // Values: ASCII, 8859/1, UNICODE, etc.

    // Heuristic: check for invalid ASCII bytes
    for _, b := range raw {
        if b > 127 {
            // Could be UTF-8 or ISO-8859-1
            return "UTF-8" // Assume UTF-8, may need detection
        }
    }

    return "ASCII"
}
```

## Z-Segment Handling Strategy

### Common Z-Segments by Vendor

#### Epic
```
ZPD - Patient Demographics Extension
ZVN - Visit Extension
ZIN - Insurance Extension
ZPM - Problem List
ZAL - Allergy Extension
```

#### Cerner
```
ZVN - Visit Number Extension
ZPD - Patient Data Extension
ZSG - Signature
```

#### Meditech
```
ZPI - Patient Info Extension
ZFI - Financial Info
```

### Generic Z-Segment Extraction

```go
type ZSegment struct {
    ID     string            `json:"id"`     // e.g., "ZPD"
    Fields []string          `json:"fields"` // Raw field values
    Parsed map[string]string `json:"parsed"` // If mapping configured
}

func extractZSegments(msg *Message) []ZSegment {
    var zsegs []ZSegment
    for _, seg := range msg.Segments {
        if strings.HasPrefix(seg.ID, "Z") {
            zsegs = append(zsegs, ZSegment{
                ID:     seg.ID,
                Fields: seg.Fields[1:], // Skip segment ID
            })
        }
    }
    return zsegs
}
```

### Z-Segment Mapping Configuration

```yaml
# Source-specific Z-segment mappings
sources:
  epic_adt:
    z_segments:
      ZPD:
        - field: 1
          target: patient.custom.mrn_checksum
          type: string
        - field: 2
          target: patient.custom.vip_flag
          type: boolean

      ZVN:
        - field: 1
          target: encounter.custom.visit_type_detail
        - field: 2
          target: encounter.custom.expected_los

  cerner_adt:
    z_segments:
      ZVN:
        - field: 1
          target: encounter.custom.cerner_visit_id
```

## Trigger Event Variations

### ADT Events: Reality vs Spec

| Event | Spec Definition | Common Variations |
|-------|-----------------|-------------------|
| A01 | Admit inpatient | Also used for OP registration |
| A04 | Register outpatient | Sometimes sent as A01 |
| A08 | Update patient | Catch-all for changes |
| A11 | Cancel admit | Some systems use A13 |
| A28 | Add person info | vs A31 update person |

### Disambiguating ADT Events

```go
func classifyAdmit(msg *Message) string {
    pv1 := msg.GetSegment("PV1")
    if pv1 == nil {
        return "unknown"
    }

    patientClass := pv1.Field(2)

    switch patientClass {
    case "I":
        return "inpatient_admit"
    case "O":
        return "outpatient_registration"
    case "E":
        return "emergency_registration"
    case "P":
        return "preadmit"
    case "R":
        return "recurring_patient"
    default:
        return "admit_unknown_class"
    }
}
```

### ORU Event Variations

```
ORU^R01: Standard result
ORU^R30: Unsolicited lab (v2.5+)
ORU^R31: Unsolicited point-of-care (v2.5+)

Some labs send ORU^R01 for everything, regardless of context.
```

## Segment Optionality Issues

### Required vs Optional Reality

```
Spec says PV1 required for ADT...
Reality: Small clinics often omit PV1-3 (location)

Spec says OBR required before OBX in ORU...
Reality: Some systems send OBX without OBR

Spec says NTE follows specific segments...
Reality: NTE can appear almost anywhere
```

### Handling Missing Segments

```go
func (p *Parser) extractPatient(msg *Message) (Patient, error) {
    pid := msg.GetSegment("PID")
    if pid == nil {
        return Patient{}, errors.New("PID segment required")
    }

    // PD1 is optional - don't fail if missing
    pd1 := msg.GetSegment("PD1")

    patient := Patient{
        MRN:        p.getField(pid, 3),
        FamilyName: p.getComponent(p.getField(pid, 5), 0),
        // ...
    }

    // Enrich from PD1 if present
    if pd1 != nil {
        pcp := p.getField(pd1, 4)
        if pcp != "" {
            patient.PrimaryCareProvider = p.parseXCN(pcp)
        }
    }

    return patient, nil
}
```

## Field Repetition Handling

### Repeating Fields in PID

```
PID-3 (Patient ID): Often repeats
PID|1||123^^^HOSP_A^MRN~456^^^HOSP_B^MRN~789^^^PAYER^PI||...

PID-11 (Address): May repeat
PID|...|123 MAIN ST^^CITY^ST^12345^USA^H~PO BOX 1^^CITY^ST^12345^USA^M|...

PID-13 (Phone): Often repeats
PID|...|^PRN^PH^^1^555^1234567~^WPN^PH^^1^555^7654321|...
```

```go
func (p *Parser) extractAllIdentifiers(pidField string) []Identifier {
    // Split on repetition separator (~)
    reps := strings.Split(pidField, "~")

    var ids []Identifier
    for _, rep := range reps {
        if rep == "" {
            continue
        }

        components := strings.Split(rep, "^")
        ids = append(ids, Identifier{
            Value:    p.getComponent(rep, 0),
            Assigner: p.getComponent(rep, 3),
            Type:     p.getComponent(rep, 4),
        })
    }

    return ids
}
```

## NULL vs Empty Field Handling

### The Semantic Difference

```
||     = Empty (no value provided)
|""|   = Explicitly empty string
|""^""|= Explicitly empty components
```

### Handling Strategy

```go
const (
    ValueEmpty       = ""     // No value
    ValueNull        = "\"\""  // Explicitly null
    ValueNotProvided = "NP"   // Legacy "not provided"
)

func interpretFieldValue(value string) (string, bool) {
    switch value {
    case "":
        return "", false // Empty, not provided
    case "\"\"":
        return "", true  // Explicitly null
    default:
        return value, true // Has value
    }
}
```

## Date/Time Format Variations

### HL7 Date/Time Formats

```
Date only:     YYYYMMDD (20240115)
Date+Time:     YYYYMMDDHHMMSS (20240115143000)
With fraction: YYYYMMDDHHMMSS.SSSS (20240115143000.1234)
With timezone: YYYYMMDDHHMMSS+/-ZZZZ (20240115143000-0500)
```

### Parsing Variations

```go
var hl7DateFormats = []string{
    "20060102150405.0000-0700", // Full with fractional and TZ
    "20060102150405-0700",      // With timezone
    "20060102150405.0000",      // With fractional seconds
    "20060102150405",           // Standard datetime
    "200601021504",             // Without seconds
    "20060102",                 // Date only
}

func parseHL7DateTime(s string, loc *time.Location) (time.Time, error) {
    s = strings.TrimSpace(s)
    if s == "" {
        return time.Time{}, nil
    }

    for _, format := range hl7DateFormats {
        if t, err := time.ParseInLocation(format, s, loc); err == nil {
            return t, nil
        }
    }

    return time.Time{}, fmt.Errorf("unparseable datetime: %s", s)
}
```

## Real-World Message Samples

### Minimal Valid ADT

```
MSH|^~\&|SRC||DEST||20240115||ADT^A01|1|P|2.3
EVN|A01|20240115
PID|||123||DOE^JOHN
PV1||I
```

### Fully-Loaded ADT (Epic-style)

```
MSH|^~\&|EPIC|HOSP|RHAPSODY|DEST|20240115080000||ADT^A01^ADT_A01|MSG001|P|2.5.1|||AL|NE|USA||
EVN|A01|20240115080000|||JSMITH^SMITH^JANE^M^RN^^LOCAL^L^^^EMP
PID|1||123456789^^^HOSP^MRN~111-22-3333^^^SSA^SS||DOE^JOHN^WILLIAM^JR^MR^||19650315000000|M||2106-3^White^HL70005|123 MAIN ST^^ANYTOWN^VA^24101^USA^H^^COUNTY||^PRN^PH^jdoe@email.com^^555^1234567~^WPN^PH^^^555^9876543||ENG|M|CHR|ACCT123456789|||N|Y|1||USA|||0
PD1||||1234567890^SMITH^JANE^M^^^MD^^NPI^L^^^DN
NK1|1|DOE^JANE^M^^MRS|SPO^Spouse^HL70063|123 MAIN ST^^ANYTOWN^VA^24101^USA|(555)1234567^PRN^PH|(555)9876543^WPN^PH|EC^Emergency Contact^HL70131
PV1|1|I|ICU^101^A^HOSP^^^^^^^^DEPID^ROOMDESC||||1234567890^SMITH^JANE^M^^^MD^^NPI^L^^^DN||MED||||7|||1234567890^SMITH^JANE^M^^^MD^^NPI^L^^^DN|IP||BCBS||||||||||||||||||||HOSP|A|||20240115080000|
PV2|||^CHEST PAIN|||||||||||||||||||||||N|N|
GT1|1||DOE^JOHN^W^^MR||123 MAIN ST^^ANYTOWN^VA^24101|(555)1234567||19650315|M||P||111-22-3333||||EMPLOYER NAME|987 WORK ST^^WORKTOWN^VA^24102|(555)5555555||FT
IN1|1|BCBS^BLUE CROSS BLUE SHIELD|123456|BCBS OF VIRGINIA|PO BOX 1^^RICHMOND^VA^23218||^WPN^PH^^^804^5551234|GROUP123|||20240101|||PPO|DOE^JOHN^W^^MR|SELF|19650315|123 MAIN ST^^ANYTOWN^VA^24101||||||||||||||||BCBS123456789||||||FT
ZPD|Y|STANDARD|GREEN|||
```

## Implementation Plan

### Phase 1: Core Parsing
- [x] Basic segment/field extraction
- [x] Delimiter detection
- [ ] Version detection from MSH-12
- [ ] Encoding character handling

### Phase 2: Version-Aware Parsing
- [ ] Data type parsers per version
- [ ] Component count validation
- [ ] Segment optionality by version

### Phase 3: Z-Segment Framework
- [ ] Generic Z-segment extraction
- [ ] Configurable field mapping
- [ ] Vendor profile support

### Phase 4: Edge Case Handling
- [ ] Escape sequence processing
- [ ] Character encoding detection
- [ ] Field repetition parsing
- [ ] NULL value semantics

## Testing Matrix

| Scenario | Test Case | Expected Result |
|----------|-----------|-----------------|
| Version 2.3 minimal | MSH only v2.3 | Parse succeeds |
| Version 2.5.1 full | All segments | All fields extracted |
| Missing PV1 | ADT without PV1 | Graceful handling |
| Z-segments | ZPD, ZVN present | Extracted to custom |
| Repeating PID-3 | Multiple MRNs | All identifiers parsed |
| Escaped delimiters | \F\ in field | Literal \| in value |
| UTF-8 characters | Name with accents | Preserved correctly |

## References

- [HL7 v2.5.1 Specification](http://www.hl7.eu/refactored/index.html)
- [HL7 Table 0203 - Identifier Type](https://hl7-definition.caristix.com/v2/HL7v2.5/Tables/0203)
- [HL7 Data Types](https://hl7-definition.caristix.com/v2/HL7v2.5/DataTypes)
