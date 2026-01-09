package edi

// BuildHLTree builds a hierarchical tree from HL segments in a transaction
func BuildHLTree(tx *Transaction) map[string]*HLNode {
	nodes := make(map[string]*HLNode)

	hlSegments := tx.GetSegments("HL")

	// First pass: create all nodes
	for _, seg := range hlSegments {
		node := &HLNode{
			ID:        seg.GetElement(1),
			ParentID:  seg.GetElement(2),
			LevelCode: seg.GetElement(3),
		}
		// HL04: 1 = has children, 0 = no children
		node.HasChildren = seg.GetElement(4) == "1"
		nodes[node.ID] = node
	}

	// Second pass: link parents and children
	for _, node := range nodes {
		if node.ParentID != "" {
			parent := nodes[node.ParentID]
			if parent != nil {
				parent.Children = append(parent.Children, node)
			}
		}
	}

	return nodes
}

// AssignSegmentsToHL assigns segments to their corresponding HL nodes
func AssignSegmentsToHL(tx *Transaction) map[string]*HLNode {
	nodes := BuildHLTree(tx)

	var currentHL *HLNode

	for _, seg := range tx.Segments {
		if seg.ID == "HL" {
			// Switch to new HL level
			hlID := seg.GetElement(1)
			currentHL = nodes[hlID]
			continue
		}

		// Assign segment to current HL level
		if currentHL != nil {
			currentHL.Segments = append(currentHL.Segments, seg)
		}
	}

	return nodes
}

// LoopStateMachine tracks the current position in the X12 loop structure
type LoopStateMachine struct {
	CurrentLoop string
	LoopStack   []string
	Transaction *Transaction
}

// NewLoopStateMachine creates a loop state machine for a transaction
func NewLoopStateMachine(tx *Transaction) *LoopStateMachine {
	return &LoopStateMachine{
		Transaction: tx,
	}
}

// Loop837Structure represents the parsed structure of an 837 transaction
type Loop837Structure struct {
	BHT              *Segment
	Submitter        *Loop1000 // 1000A
	Receiver         *Loop1000 // 1000B
	BillingProviders []*Loop2000A
}

// Loop1000 represents the 1000A/1000B submitter/receiver loops
type Loop1000 struct {
	NM1 *Segment
	PER []*Segment
}

// Loop2000A represents the Billing Provider HL loop
type Loop2000A struct {
	HL           *Segment
	PRV          *Segment
	BillingName  *Loop2010 // 2010AA
	PayToAddress *Loop2010 // 2010AB
	Subscribers  []*Loop2000B
}

// Loop2000B represents the Subscriber HL loop
type Loop2000B struct {
	HL             *Segment
	SBR            *Segment
	SubscriberInfo *Loop2010 // 2010BA
	PayerInfo      *Loop2010 // 2010BB
	Patient        *Loop2000C
	Claims         []*Loop2300
}

// Loop2000C represents the Patient HL loop (when patient != subscriber)
type Loop2000C struct {
	HL          *Segment
	PAT         *Segment
	PatientInfo *Loop2010 // 2010CA
}

// Loop2010 represents an NM1 loop with associated address and reference segments
type Loop2010 struct {
	NM1 *Segment
	N3  *Segment
	N4  *Segment
	REF []*Segment
	PER []*Segment
	DMG *Segment
}

// Loop2300 represents a Claim Information loop
type Loop2300 struct {
	CLM              *Segment
	DTP              []*Segment
	CL1              *Segment // Institutional only
	PWK              []*Segment
	CN1              *Segment
	AMT              []*Segment
	REF              []*Segment
	K3               []*Segment
	NTE              []*Segment
	CR1              *Segment
	CR2              *Segment
	CRC              []*Segment
	HI               []*Segment // Diagnosis codes
	HCP              *Segment
	Providers        []*Loop2310
	OtherSubscribers []*Loop2320
	ServiceLines     []*Loop2400
}

// Loop2310 represents a provider loop within a claim (Referring, Rendering, etc.)
type Loop2310 struct {
	LoopID string // 2310A, 2310B, etc.
	NM1    *Segment
	PRV    *Segment
	REF    []*Segment
	N3     *Segment
	N4     *Segment
}

// Loop2320 represents Other Subscriber Information (COB)
type Loop2320 struct {
	SBR *Segment
	CAS []*Segment
	AMT []*Segment
	DMG *Segment
	OI  *Segment
	MOA *Segment
	NM1 []*Segment
}

// Loop2400 represents a Service Line
type Loop2400 struct {
	LX            *Segment
	SV1           *Segment // Professional service
	SV2           *Segment // Institutional service
	SV3           *Segment // Dental service
	DTP           []*Segment
	PWK           []*Segment
	REF           []*Segment
	AMT           []*Segment
	K3            []*Segment
	NTE           []*Segment
	PS1           *Segment
	HCP           *Segment
	Providers     []*Loop2420
	Adjudications []*Loop2430
}

// Loop2420 represents line-level provider information
type Loop2420 struct {
	NM1 *Segment
	PRV *Segment
	REF []*Segment
}

// Loop2430 represents line adjudication (from 835 cross-reference)
type Loop2430 struct {
	SVD *Segment
	CAS []*Segment
	DTP *Segment
	AMT *Segment
}

// Parse837Loops parses an 837 transaction into its loop structure
func Parse837Loops(tx *Transaction) *Loop837Structure {
	result := &Loop837Structure{}

	// Build HL hierarchy
	hlNodes := AssignSegmentsToHL(tx)

	// Process non-HL segments at the beginning
	state := "header"
	var currentLoop1000 *Loop1000
	var current2000A *Loop2000A
	var current2000B *Loop2000B
	var current2300 *Loop2300
	var current2400 *Loop2400

	for _, seg := range tx.Segments {
		switch seg.ID {
		case "BHT":
			result.BHT = seg
			state = "1000A"

		case "NM1":
			nm1Type := seg.GetElement(1)
			switch state {
			case "1000A":
				if nm1Type == "41" {
					currentLoop1000 = &Loop1000{NM1: seg}
					result.Submitter = currentLoop1000
					state = "1000A_content"
				}
			case "1000A_content", "1000B":
				if nm1Type == "40" {
					currentLoop1000 = &Loop1000{NM1: seg}
					result.Receiver = currentLoop1000
					state = "1000B_content"
				}
			case "2010AA":
				current2000A.BillingName = &Loop2010{NM1: seg}
				state = "2010AA_content"
			case "2010AB":
				current2000A.PayToAddress = &Loop2010{NM1: seg}
				state = "2010AB_content"
			case "2010BA":
				current2000B.SubscriberInfo = &Loop2010{NM1: seg}
				state = "2010BA_content"
			case "2010BA_content", "2010BB":
				if nm1Type == "PR" {
					current2000B.PayerInfo = &Loop2010{NM1: seg}
					state = "2010BB_content"
				}
			case "2300", "2300_content", "2310":
				// Provider NM1 within claim
				if current2300 != nil {
					provider := &Loop2310{NM1: seg}
					switch nm1Type {
					case "DN", "P3":
						provider.LoopID = "2310A" // Referring provider
					case "82":
						provider.LoopID = "2310B" // Rendering provider
					case "77":
						provider.LoopID = "2310C" // Service facility
					case "DQ":
						provider.LoopID = "2310D" // Supervising provider
					default:
						provider.LoopID = "2310"
					}
					current2300.Providers = append(current2300.Providers, provider)
					state = "2310"
				}
			}

		case "PER":
			if currentLoop1000 != nil && (state == "1000A_content" || state == "1000B_content") {
				currentLoop1000.PER = append(currentLoop1000.PER, seg)
			}

		case "HL":
			hlCode := seg.GetElement(3)
			hlID := seg.GetElement(1)
			node := hlNodes[hlID]

			switch hlCode {
			case HLLevelInformationSource: // 20 - Billing Provider
				current2000A = &Loop2000A{HL: seg}
				if node != nil {
					// Copy segments already assigned to this HL
					for _, s := range node.Segments {
						if s.ID == "PRV" {
							current2000A.PRV = s
						}
					}
				}
				result.BillingProviders = append(result.BillingProviders, current2000A)
				state = "2010AA"

			case HLLevelSubscriber: // 22 - Subscriber
				current2000B = &Loop2000B{HL: seg}
				if current2000A != nil {
					current2000A.Subscribers = append(current2000A.Subscribers, current2000B)
				}
				state = "2010BA"

			case HLLevelDependent: // 23 - Patient
				if current2000B != nil {
					current2000B.Patient = &Loop2000C{HL: seg}
				}
			}

		case "SBR":
			if current2000B != nil && state == "2010BA" {
				current2000B.SBR = seg
			}

		case "N3":
			handleN3(state, seg, current2000A, current2000B)

		case "N4":
			handleN4(state, seg, current2000A, current2000B)

		case "REF":
			handleREF(state, seg, current2000A, current2000B, current2300)

		case "DMG":
			if current2000B != nil && current2000B.SubscriberInfo != nil {
				current2000B.SubscriberInfo.DMG = seg
			}

		case "CLM":
			current2300 = &Loop2300{CLM: seg}
			if current2000B != nil {
				current2000B.Claims = append(current2000B.Claims, current2300)
			}
			state = "2300_content"

		case "HI":
			if current2300 != nil {
				current2300.HI = append(current2300.HI, seg)
			}

		case "DTP":
			if current2400 != nil {
				current2400.DTP = append(current2400.DTP, seg)
			} else if current2300 != nil {
				current2300.DTP = append(current2300.DTP, seg)
			}

		case "LX":
			current2400 = &Loop2400{LX: seg}
			if current2300 != nil {
				current2300.ServiceLines = append(current2300.ServiceLines, current2400)
			}
			state = "2400"

		case "SV1":
			if current2400 != nil {
				current2400.SV1 = seg
			}

		case "SV2":
			if current2400 != nil {
				current2400.SV2 = seg
			}
		}
	}

	return result
}

func handleN3(state string, seg *Segment, current2000A *Loop2000A, current2000B *Loop2000B) {
	switch state {
	case "2010AA_content":
		if current2000A != nil && current2000A.BillingName != nil {
			current2000A.BillingName.N3 = seg
		}
	case "2010BA_content":
		if current2000B != nil && current2000B.SubscriberInfo != nil {
			current2000B.SubscriberInfo.N3 = seg
		}
	}
}

func handleN4(state string, seg *Segment, current2000A *Loop2000A, current2000B *Loop2000B) {
	switch state {
	case "2010AA_content":
		if current2000A != nil && current2000A.BillingName != nil {
			current2000A.BillingName.N4 = seg
		}
	case "2010BA_content":
		if current2000B != nil && current2000B.SubscriberInfo != nil {
			current2000B.SubscriberInfo.N4 = seg
		}
	}
}

func handleREF(state string, seg *Segment, current2000A *Loop2000A, current2000B *Loop2000B, current2300 *Loop2300) {
	switch state {
	case "2010AA_content":
		if current2000A != nil && current2000A.BillingName != nil {
			current2000A.BillingName.REF = append(current2000A.BillingName.REF, seg)
		}
	case "2300_content":
		if current2300 != nil {
			current2300.REF = append(current2300.REF, seg)
		}
	}
}

// Loop835Structure represents the parsed structure of an 835 transaction
type Loop835Structure struct {
	BPR     *Segment // Beginning of Payment
	TRN     *Segment // Check/EFT trace
	DTM     []*Segment
	Payer   *Loop1000 // 1000A
	Payee   *Loop1000 // 1000B
	Headers []*Loop835Header
}

// Loop835Header represents a header number level (2000)
type Loop835Header struct {
	LX     *Segment
	TS3    *Segment
	TS2    *Segment
	Claims []*Loop835Claim
}

// Loop835Claim represents claim payment information (2100)
type Loop835Claim struct {
	CLP          *Segment
	CAS          []*Segment
	NM1          []*Segment
	MIA          *Segment // Inpatient adjudication
	MOA          *Segment
	REF          []*Segment
	DTM          []*Segment
	PER          []*Segment
	AMT          []*Segment
	QTY          []*Segment
	ServiceLines []*Loop835Service
}

// Loop835Service represents service line payment (2110)
type Loop835Service struct {
	SVC *Segment
	DTM []*Segment
	CAS []*Segment
	REF []*Segment
	AMT []*Segment
	QTY []*Segment
	LQ  []*Segment
}

// Loop270Structure represents the parsed structure of a 270 eligibility inquiry
type Loop270Structure struct {
	BHT                 *Segment
	InformationSources  []*Loop270Source // 2000A - Information Source (Payer)
}

// Loop270Source represents the Information Source (Payer) hierarchy
type Loop270Source struct {
	HL              *Segment
	SourceInfo      *Loop270Entity // 2100A - Source Name
	Receivers       []*Loop270Receiver
}

// Loop270Receiver represents the Information Receiver (Provider) hierarchy
type Loop270Receiver struct {
	HL           *Segment
	ReceiverInfo *Loop270Entity // 2100B - Receiver Name
	Subscribers  []*Loop270Subscriber
}

// Loop270Subscriber represents the Subscriber hierarchy
type Loop270Subscriber struct {
	HL             *Segment
	TRN            []*Segment // Trace numbers
	SubscriberInfo *Loop270Entity // 2100C - Subscriber Name
	EligibilityReq []*Loop270Eligibility // 2110C - Eligibility inquiries
	Dependents     []*Loop270Dependent
}

// Loop270Dependent represents a Dependent (if different from subscriber)
type Loop270Dependent struct {
	HL             *Segment
	TRN            []*Segment
	DependentInfo  *Loop270Entity // 2100D - Dependent Name
	EligibilityReq []*Loop270Eligibility // 2110D - Eligibility inquiries
}

// Loop270Entity holds NM1-based entity information (used in 270/271)
type Loop270Entity struct {
	NM1 *Segment
	REF []*Segment
	N3  *Segment
	N4  *Segment
	PER []*Segment
	DMG *Segment
	INS *Segment
	HI  []*Segment
	DTP []*Segment
}

// Loop270Eligibility represents an eligibility/inquiry loop (2110C/2110D)
type Loop270Eligibility struct {
	EQ  *Segment // Eligibility or Benefit Inquiry
	III []*Segment // Additional Information
	REF []*Segment
	DTP []*Segment
}

// Loop271Structure represents the parsed structure of a 271 eligibility response
type Loop271Structure struct {
	BHT                 *Segment
	InformationSources  []*Loop271Source // 2000A - Information Source (Payer)
}

// Loop271Source represents the Information Source hierarchy in 271
type Loop271Source struct {
	HL              *Segment
	AAA             []*Segment // Request validation
	SourceInfo      *Loop271Entity // 2100A - Source Name
	Receivers       []*Loop271Receiver
}

// Loop271Receiver represents the Information Receiver hierarchy in 271
type Loop271Receiver struct {
	HL           *Segment
	ReceiverInfo *Loop271Entity // 2100B - Receiver Name
	Subscribers  []*Loop271Subscriber
}

// Loop271Subscriber represents the Subscriber hierarchy in 271
type Loop271Subscriber struct {
	HL             *Segment
	TRN            []*Segment
	SubscriberInfo *Loop271Entity // 2100C - Subscriber Name
	Benefits       []*Loop271Benefit // 2110C - Eligibility/Benefit Information
	Dependents     []*Loop271Dependent
}

// Loop271Dependent represents a Dependent in 271
type Loop271Dependent struct {
	HL            *Segment
	TRN           []*Segment
	DependentInfo *Loop271Entity // 2100D - Dependent Name
	Benefits      []*Loop271Benefit // 2110D - Eligibility/Benefit Information
}

// Loop271Entity holds NM1-based entity information (used in 271)
type Loop271Entity struct {
	NM1 *Segment
	REF []*Segment
	N3  *Segment
	N4  *Segment
	PER []*Segment
	AAA []*Segment // Request validation at entity level
	DMG *Segment
	INS *Segment
	HI  []*Segment
	DTP []*Segment
}

// Loop271Benefit represents eligibility/benefit information (2110C/2110D)
type Loop271Benefit struct {
	EB  *Segment // Eligibility or Benefit Information
	HSD []*Segment // Health Care Services Delivery
	REF []*Segment
	DTP []*Segment
	AAA []*Segment // Request validation
	MSG []*Segment // Message Text
	III []*Segment // Additional Information
	LS  *Segment // Loop Header
	LE  *Segment // Loop Trailer
}

// Parse270Loops parses a 270 transaction into its loop structure
func Parse270Loops(tx *Transaction) *Loop270Structure {
	result := &Loop270Structure{}

	// Build HL hierarchy
	hlNodes := AssignSegmentsToHL(tx)

	var currentSource *Loop270Source
	var currentReceiver *Loop270Receiver
	var currentSubscriber *Loop270Subscriber
	var currentDependent *Loop270Dependent
	var currentEntity *Loop270Entity
	var currentEligibility *Loop270Eligibility
	state := "header"

	for _, seg := range tx.Segments {
		switch seg.ID {
		case "BHT":
			result.BHT = seg
			state = "bht"

		case "HL":
			hlCode := seg.GetElement(3)
			hlID := seg.GetElement(1)
			_ = hlNodes[hlID] // Reference to ensure HL is valid

			switch hlCode {
			case HLLevelInformationSource: // 20 - Information Source (Payer)
				currentSource = &Loop270Source{HL: seg}
				result.InformationSources = append(result.InformationSources, currentSource)
				currentReceiver = nil
				currentSubscriber = nil
				currentDependent = nil
				currentEntity = nil
				currentEligibility = nil
				state = "2000A"

			case HLLevelInformationReceiver: // 21 - Information Receiver (Provider)
				currentReceiver = &Loop270Receiver{HL: seg}
				if currentSource != nil {
					currentSource.Receivers = append(currentSource.Receivers, currentReceiver)
				}
				currentSubscriber = nil
				currentDependent = nil
				currentEntity = nil
				currentEligibility = nil
				state = "2000B"

			case HLLevelSubscriber: // 22 - Subscriber
				currentSubscriber = &Loop270Subscriber{HL: seg}
				if currentReceiver != nil {
					currentReceiver.Subscribers = append(currentReceiver.Subscribers, currentSubscriber)
				}
				currentDependent = nil
				currentEntity = nil
				currentEligibility = nil
				state = "2000C"

			case HLLevelDependent: // 23 - Dependent
				currentDependent = &Loop270Dependent{HL: seg}
				if currentSubscriber != nil {
					currentSubscriber.Dependents = append(currentSubscriber.Dependents, currentDependent)
				}
				currentEntity = nil
				currentEligibility = nil
				state = "2000D"
			}

		case "TRN":
			if currentDependent != nil {
				currentDependent.TRN = append(currentDependent.TRN, seg)
			} else if currentSubscriber != nil {
				currentSubscriber.TRN = append(currentSubscriber.TRN, seg)
			}

		case "NM1":
			currentEntity = &Loop270Entity{NM1: seg}
			switch state {
			case "2000A":
				currentSource.SourceInfo = currentEntity
				state = "2100A"
			case "2000B":
				currentReceiver.ReceiverInfo = currentEntity
				state = "2100B"
			case "2000C":
				currentSubscriber.SubscriberInfo = currentEntity
				state = "2100C"
			case "2000D":
				currentDependent.DependentInfo = currentEntity
				state = "2100D"
			}

		case "REF":
			if currentEligibility != nil {
				currentEligibility.REF = append(currentEligibility.REF, seg)
			} else if currentEntity != nil {
				currentEntity.REF = append(currentEntity.REF, seg)
			}

		case "N3":
			if currentEntity != nil {
				currentEntity.N3 = seg
			}

		case "N4":
			if currentEntity != nil {
				currentEntity.N4 = seg
			}

		case "PER":
			if currentEntity != nil {
				currentEntity.PER = append(currentEntity.PER, seg)
			}

		case "DMG":
			if currentEntity != nil {
				currentEntity.DMG = seg
			}

		case "INS":
			if currentEntity != nil {
				currentEntity.INS = seg
			}

		case "HI":
			if currentEntity != nil {
				currentEntity.HI = append(currentEntity.HI, seg)
			}

		case "DTP":
			if currentEligibility != nil {
				currentEligibility.DTP = append(currentEligibility.DTP, seg)
			} else if currentEntity != nil {
				currentEntity.DTP = append(currentEntity.DTP, seg)
			}

		case "EQ":
			currentEligibility = &Loop270Eligibility{EQ: seg}
			if state == "2100D" && currentDependent != nil {
				currentDependent.EligibilityReq = append(currentDependent.EligibilityReq, currentEligibility)
			} else if currentSubscriber != nil {
				currentSubscriber.EligibilityReq = append(currentSubscriber.EligibilityReq, currentEligibility)
			}
			if state == "2100C" {
				state = "2110C"
			} else if state == "2100D" {
				state = "2110D"
			}

		case "III":
			if currentEligibility != nil {
				currentEligibility.III = append(currentEligibility.III, seg)
			}
		}
	}

	return result
}

// Parse271Loops parses a 271 transaction into its loop structure
func Parse271Loops(tx *Transaction) *Loop271Structure {
	result := &Loop271Structure{}

	// Build HL hierarchy
	hlNodes := AssignSegmentsToHL(tx)

	var currentSource *Loop271Source
	var currentReceiver *Loop271Receiver
	var currentSubscriber *Loop271Subscriber
	var currentDependent *Loop271Dependent
	var currentEntity *Loop271Entity
	var currentBenefit *Loop271Benefit
	state := "header"

	for _, seg := range tx.Segments {
		switch seg.ID {
		case "BHT":
			result.BHT = seg
			state = "bht"

		case "HL":
			hlCode := seg.GetElement(3)
			hlID := seg.GetElement(1)
			_ = hlNodes[hlID]

			switch hlCode {
			case HLLevelInformationSource: // 20 - Information Source (Payer)
				currentSource = &Loop271Source{HL: seg}
				result.InformationSources = append(result.InformationSources, currentSource)
				currentReceiver = nil
				currentSubscriber = nil
				currentDependent = nil
				currentEntity = nil
				currentBenefit = nil
				state = "2000A"

			case HLLevelInformationReceiver: // 21 - Information Receiver (Provider)
				currentReceiver = &Loop271Receiver{HL: seg}
				if currentSource != nil {
					currentSource.Receivers = append(currentSource.Receivers, currentReceiver)
				}
				currentSubscriber = nil
				currentDependent = nil
				currentEntity = nil
				currentBenefit = nil
				state = "2000B"

			case HLLevelSubscriber: // 22 - Subscriber
				currentSubscriber = &Loop271Subscriber{HL: seg}
				if currentReceiver != nil {
					currentReceiver.Subscribers = append(currentReceiver.Subscribers, currentSubscriber)
				}
				currentDependent = nil
				currentEntity = nil
				currentBenefit = nil
				state = "2000C"

			case HLLevelDependent: // 23 - Dependent
				currentDependent = &Loop271Dependent{HL: seg}
				if currentSubscriber != nil {
					currentSubscriber.Dependents = append(currentSubscriber.Dependents, currentDependent)
				}
				currentEntity = nil
				currentBenefit = nil
				state = "2000D"
			}

		case "AAA":
			if currentBenefit != nil {
				currentBenefit.AAA = append(currentBenefit.AAA, seg)
			} else if currentEntity != nil {
				currentEntity.AAA = append(currentEntity.AAA, seg)
			} else if currentSource != nil && state == "2000A" {
				currentSource.AAA = append(currentSource.AAA, seg)
			}

		case "TRN":
			if currentDependent != nil {
				currentDependent.TRN = append(currentDependent.TRN, seg)
			} else if currentSubscriber != nil {
				currentSubscriber.TRN = append(currentSubscriber.TRN, seg)
			}

		case "NM1":
			currentEntity = &Loop271Entity{NM1: seg}
			currentBenefit = nil // Reset benefit when entering new entity
			switch state {
			case "2000A":
				currentSource.SourceInfo = currentEntity
				state = "2100A"
			case "2000B":
				currentReceiver.ReceiverInfo = currentEntity
				state = "2100B"
			case "2000C":
				currentSubscriber.SubscriberInfo = currentEntity
				state = "2100C"
			case "2000D":
				currentDependent.DependentInfo = currentEntity
				state = "2100D"
			}

		case "REF":
			if currentBenefit != nil {
				currentBenefit.REF = append(currentBenefit.REF, seg)
			} else if currentEntity != nil {
				currentEntity.REF = append(currentEntity.REF, seg)
			}

		case "N3":
			if currentEntity != nil {
				currentEntity.N3 = seg
			}

		case "N4":
			if currentEntity != nil {
				currentEntity.N4 = seg
			}

		case "PER":
			if currentEntity != nil {
				currentEntity.PER = append(currentEntity.PER, seg)
			}

		case "DMG":
			if currentEntity != nil {
				currentEntity.DMG = seg
			}

		case "INS":
			if currentEntity != nil {
				currentEntity.INS = seg
			}

		case "HI":
			if currentEntity != nil {
				currentEntity.HI = append(currentEntity.HI, seg)
			}

		case "DTP":
			if currentBenefit != nil {
				currentBenefit.DTP = append(currentBenefit.DTP, seg)
			} else if currentEntity != nil {
				currentEntity.DTP = append(currentEntity.DTP, seg)
			}

		case "EB":
			currentBenefit = &Loop271Benefit{EB: seg}
			if state == "2100D" && currentDependent != nil {
				currentDependent.Benefits = append(currentDependent.Benefits, currentBenefit)
				state = "2110D"
			} else if currentSubscriber != nil {
				currentSubscriber.Benefits = append(currentSubscriber.Benefits, currentBenefit)
				state = "2110C"
			}

		case "HSD":
			if currentBenefit != nil {
				currentBenefit.HSD = append(currentBenefit.HSD, seg)
			}

		case "MSG":
			if currentBenefit != nil {
				currentBenefit.MSG = append(currentBenefit.MSG, seg)
			}

		case "III":
			if currentBenefit != nil {
				currentBenefit.III = append(currentBenefit.III, seg)
			}

		case "LS":
			if currentBenefit != nil {
				currentBenefit.LS = seg
			}

		case "LE":
			if currentBenefit != nil {
				currentBenefit.LE = seg
			}
		}
	}

	return result
}

// Parse835Loops parses an 835 transaction into its loop structure
func Parse835Loops(tx *Transaction) *Loop835Structure {
	result := &Loop835Structure{}

	var currentHeader *Loop835Header
	var currentClaim *Loop835Claim
	var currentService *Loop835Service
	state := "header"

	for _, seg := range tx.Segments {
		switch seg.ID {
		case "BPR":
			result.BPR = seg

		case "TRN":
			result.TRN = seg

		case "DTM":
			if state == "header" {
				result.DTM = append(result.DTM, seg)
			} else if currentService != nil {
				currentService.DTM = append(currentService.DTM, seg)
			} else if currentClaim != nil {
				currentClaim.DTM = append(currentClaim.DTM, seg)
			}

		case "N1":
			n1Type := seg.GetElement(1)
			switch n1Type {
			case "PR": // Payer
				result.Payer = &Loop1000{NM1: seg}
				state = "1000A"
			case "PE": // Payee
				result.Payee = &Loop1000{NM1: seg}
				state = "1000B"
			}

		case "LX":
			currentHeader = &Loop835Header{LX: seg}
			result.Headers = append(result.Headers, currentHeader)
			state = "2000"
			currentClaim = nil
			currentService = nil

		case "TS3":
			if currentHeader != nil {
				currentHeader.TS3 = seg
			}

		case "TS2":
			if currentHeader != nil {
				currentHeader.TS2 = seg
			}

		case "CLP":
			currentClaim = &Loop835Claim{CLP: seg}
			if currentHeader != nil {
				currentHeader.Claims = append(currentHeader.Claims, currentClaim)
			}
			state = "2100"
			currentService = nil

		case "CAS":
			if currentService != nil {
				currentService.CAS = append(currentService.CAS, seg)
			} else if currentClaim != nil {
				currentClaim.CAS = append(currentClaim.CAS, seg)
			}

		case "NM1":
			if currentClaim != nil {
				currentClaim.NM1 = append(currentClaim.NM1, seg)
			}

		case "MIA":
			if currentClaim != nil {
				currentClaim.MIA = seg
			}

		case "MOA":
			if currentClaim != nil {
				currentClaim.MOA = seg
			}

		case "REF":
			if currentService != nil {
				currentService.REF = append(currentService.REF, seg)
			} else if currentClaim != nil {
				currentClaim.REF = append(currentClaim.REF, seg)
			}

		case "AMT":
			if currentService != nil {
				currentService.AMT = append(currentService.AMT, seg)
			} else if currentClaim != nil {
				currentClaim.AMT = append(currentClaim.AMT, seg)
			}

		case "QTY":
			if currentService != nil {
				currentService.QTY = append(currentService.QTY, seg)
			} else if currentClaim != nil {
				currentClaim.QTY = append(currentClaim.QTY, seg)
			}

		case "SVC":
			currentService = &Loop835Service{SVC: seg}
			if currentClaim != nil {
				currentClaim.ServiceLines = append(currentClaim.ServiceLines, currentService)
			}
			state = "2110"

		case "LQ":
			if currentService != nil {
				currentService.LQ = append(currentService.LQ, seg)
			}

		case "PER":
			if currentClaim != nil {
				currentClaim.PER = append(currentClaim.PER, seg)
			}
		}
	}

	return result
}

// --- 276/277 Claim Status Loop Structures ---

// Loop276Structure represents the parsed structure of a 276 claim status request
type Loop276Structure struct {
	BHT                *Segment
	InformationSources []*Loop276Source // 2000A - Information Source (Payer)
}

// Loop276Source represents the Information Source (Payer) hierarchy
type Loop276Source struct {
	HL         *Segment
	SourceInfo *Loop276Entity // 2100A - Source Name
	Receivers  []*Loop276Receiver
}

// Loop276Receiver represents the Information Receiver (Provider) hierarchy
type Loop276Receiver struct {
	HL           *Segment
	ReceiverInfo *Loop276Entity // 2100B - Receiver Name
	Subscribers  []*Loop276Subscriber
}

// Loop276Subscriber represents the Subscriber hierarchy in a 276
type Loop276Subscriber struct {
	HL             *Segment
	SBR            *Segment          // Subscriber Information
	SubscriberInfo *Loop276Entity    // 2100C - Subscriber Name
	ClaimInquiries []*Loop276Inquiry // 2200C - Claim inquiries
	Dependents     []*Loop276Dependent
}

// Loop276Dependent represents a Dependent in 276
type Loop276Dependent struct {
	HL             *Segment
	DependentInfo  *Loop276Entity    // 2100D - Dependent Name
	ClaimInquiries []*Loop276Inquiry // 2200D - Claim inquiries
}

// Loop276Entity holds NM1-based entity information
type Loop276Entity struct {
	NM1 *Segment
	N3  *Segment
	N4  *Segment
	PER []*Segment
	REF []*Segment
	DMG *Segment
}

// Loop276Inquiry represents a claim status inquiry (Loop 2200)
type Loop276Inquiry struct {
	TRN          *Segment // Trace Number (required)
	REF          []*Segment
	AMT          []*Segment
	DTP          []*Segment
	ServiceLines []*Loop276ServiceLine // 2210 - Service line details
}

// Loop276ServiceLine represents service line detail in inquiry (Loop 2210)
type Loop276ServiceLine struct {
	SVC *Segment // Service identification
	REF []*Segment
	DTP []*Segment
}

// Loop277Structure represents the parsed structure of a 277 claim status response
type Loop277Structure struct {
	BHT                *Segment
	InformationSources []*Loop277Source // 2000A - Information Source (Payer)
}

// Loop277Source represents the Information Source (Payer) hierarchy
type Loop277Source struct {
	HL         *Segment
	SourceInfo *Loop277Entity // 2100A - Source Name
	Receivers  []*Loop277Receiver
}

// Loop277Receiver represents the Information Receiver (Provider) hierarchy
type Loop277Receiver struct {
	HL           *Segment
	ReceiverInfo *Loop277Entity // 2100B - Receiver Name
	Subscribers  []*Loop277Subscriber
}

// Loop277Subscriber represents the Subscriber hierarchy in a 277
type Loop277Subscriber struct {
	HL             *Segment
	SBR            *Segment         // Subscriber Information
	SubscriberInfo *Loop277Entity   // 2100C - Subscriber Name
	ClaimStatuses  []*Loop277Status // 2200C - Claim status details
	Dependents     []*Loop277Dependent
}

// Loop277Dependent represents a Dependent in 277
type Loop277Dependent struct {
	HL            *Segment
	DependentInfo *Loop277Entity   // 2100D - Dependent Name
	ClaimStatuses []*Loop277Status // 2200D - Claim status details
}

// Loop277Entity holds NM1-based entity information
type Loop277Entity struct {
	NM1 *Segment
	N3  *Segment
	N4  *Segment
	PER []*Segment
	REF []*Segment
	DMG *Segment
}

// Loop277Status represents claim status information (Loop 2200)
type Loop277Status struct {
	TRN          *Segment   // Trace Number
	STC          []*Segment // Status Information (can repeat)
	REF          []*Segment // Reference Information
	DTP          []*Segment // Date/Time Information
	QTY          []*Segment // Quantity
	AMT          []*Segment // Monetary Amounts
	ServiceLines []*Loop277ServiceLineStatus // 2220 - Service line status
}

// Loop277ServiceLineStatus represents service line status (Loop 2220)
type Loop277ServiceLineStatus struct {
	SVC *Segment   // Service identification
	STC []*Segment // Status Information for this line
	REF []*Segment
	DTP []*Segment
}

// Parse276Loops parses a 276 transaction into its loop structure
func Parse276Loops(tx *Transaction) *Loop276Structure {
	result := &Loop276Structure{}

	nodes := AssignSegmentsToHL(tx)

	var currentSource *Loop276Source
	var currentReceiver *Loop276Receiver
	var currentSubscriber *Loop276Subscriber
	var currentDependent *Loop276Dependent
	var currentEntity *Loop276Entity
	var currentInquiry *Loop276Inquiry
	var currentServiceLine *Loop276ServiceLine
	state := "header"

	for _, seg := range tx.Segments {
		switch seg.ID {
		case "BHT":
			result.BHT = seg

		case "HL":
			hlID := seg.GetElement(1)
			node := nodes[hlID]
			if node == nil {
				continue
			}
			switch node.LevelCode {
			case "20": // Information Source (Payer)
				currentSource = &Loop276Source{HL: seg}
				result.InformationSources = append(result.InformationSources, currentSource)
				state = "2000A"
				currentReceiver = nil
				currentSubscriber = nil
				currentDependent = nil
				currentEntity = nil
				currentInquiry = nil

			case "21": // Information Receiver (Provider)
				currentReceiver = &Loop276Receiver{HL: seg}
				if currentSource != nil {
					currentSource.Receivers = append(currentSource.Receivers, currentReceiver)
				}
				state = "2000B"
				currentSubscriber = nil
				currentDependent = nil
				currentEntity = nil
				currentInquiry = nil

			case "22": // Subscriber
				currentSubscriber = &Loop276Subscriber{HL: seg}
				if currentReceiver != nil {
					currentReceiver.Subscribers = append(currentReceiver.Subscribers, currentSubscriber)
				}
				state = "2000C"
				currentDependent = nil
				currentEntity = nil
				currentInquiry = nil

			case "23": // Dependent
				currentDependent = &Loop276Dependent{HL: seg}
				if currentSubscriber != nil {
					currentSubscriber.Dependents = append(currentSubscriber.Dependents, currentDependent)
				}
				state = "2000D"
				currentEntity = nil
				currentInquiry = nil
			}

		case "SBR":
			if currentSubscriber != nil {
				currentSubscriber.SBR = seg
			}

		case "NM1":
			currentEntity = &Loop276Entity{NM1: seg}
			switch state {
			case "2000A":
				if currentSource != nil {
					currentSource.SourceInfo = currentEntity
					state = "2100A"
				}
			case "2000B":
				if currentReceiver != nil {
					currentReceiver.ReceiverInfo = currentEntity
					state = "2100B"
				}
			case "2000C":
				if currentSubscriber != nil {
					currentSubscriber.SubscriberInfo = currentEntity
					state = "2100C"
				}
			case "2000D":
				if currentDependent != nil {
					currentDependent.DependentInfo = currentEntity
					state = "2100D"
				}
			}

		case "N3":
			if currentEntity != nil {
				currentEntity.N3 = seg
			}

		case "N4":
			if currentEntity != nil {
				currentEntity.N4 = seg
			}

		case "PER":
			if currentEntity != nil {
				currentEntity.PER = append(currentEntity.PER, seg)
			}

		case "DMG":
			if currentEntity != nil {
				currentEntity.DMG = seg
			}

		case "TRN":
			// TRN starts a new claim inquiry (Loop 2200)
			currentInquiry = &Loop276Inquiry{TRN: seg}
			if state == "2100D" || state == "2200D" {
				if currentDependent != nil {
					currentDependent.ClaimInquiries = append(currentDependent.ClaimInquiries, currentInquiry)
					state = "2200D"
				}
			} else if currentSubscriber != nil {
				currentSubscriber.ClaimInquiries = append(currentSubscriber.ClaimInquiries, currentInquiry)
				state = "2200C"
			}
			currentServiceLine = nil

		case "REF":
			if currentServiceLine != nil {
				currentServiceLine.REF = append(currentServiceLine.REF, seg)
			} else if currentInquiry != nil {
				currentInquiry.REF = append(currentInquiry.REF, seg)
			} else if currentEntity != nil {
				currentEntity.REF = append(currentEntity.REF, seg)
			}

		case "AMT":
			if currentInquiry != nil {
				currentInquiry.AMT = append(currentInquiry.AMT, seg)
			}

		case "DTP":
			if currentServiceLine != nil {
				currentServiceLine.DTP = append(currentServiceLine.DTP, seg)
			} else if currentInquiry != nil {
				currentInquiry.DTP = append(currentInquiry.DTP, seg)
			}

		case "SVC":
			// SVC starts a service line detail (Loop 2210)
			currentServiceLine = &Loop276ServiceLine{SVC: seg}
			if currentInquiry != nil {
				currentInquiry.ServiceLines = append(currentInquiry.ServiceLines, currentServiceLine)
			}
		}
	}

	return result
}

// Parse277Loops parses a 277 transaction into its loop structure
func Parse277Loops(tx *Transaction) *Loop277Structure {
	result := &Loop277Structure{}

	nodes := AssignSegmentsToHL(tx)

	var currentSource *Loop277Source
	var currentReceiver *Loop277Receiver
	var currentSubscriber *Loop277Subscriber
	var currentDependent *Loop277Dependent
	var currentEntity *Loop277Entity
	var currentStatus *Loop277Status
	var currentServiceLine *Loop277ServiceLineStatus
	state := "header"

	for _, seg := range tx.Segments {
		switch seg.ID {
		case "BHT":
			result.BHT = seg

		case "HL":
			hlID := seg.GetElement(1)
			node := nodes[hlID]
			if node == nil {
				continue
			}
			switch node.LevelCode {
			case "20": // Information Source (Payer)
				currentSource = &Loop277Source{HL: seg}
				result.InformationSources = append(result.InformationSources, currentSource)
				state = "2000A"
				currentReceiver = nil
				currentSubscriber = nil
				currentDependent = nil
				currentEntity = nil
				currentStatus = nil

			case "21": // Information Receiver (Provider)
				currentReceiver = &Loop277Receiver{HL: seg}
				if currentSource != nil {
					currentSource.Receivers = append(currentSource.Receivers, currentReceiver)
				}
				state = "2000B"
				currentSubscriber = nil
				currentDependent = nil
				currentEntity = nil
				currentStatus = nil

			case "22": // Subscriber
				currentSubscriber = &Loop277Subscriber{HL: seg}
				if currentReceiver != nil {
					currentReceiver.Subscribers = append(currentReceiver.Subscribers, currentSubscriber)
				}
				state = "2000C"
				currentDependent = nil
				currentEntity = nil
				currentStatus = nil

			case "23": // Dependent
				currentDependent = &Loop277Dependent{HL: seg}
				if currentSubscriber != nil {
					currentSubscriber.Dependents = append(currentSubscriber.Dependents, currentDependent)
				}
				state = "2000D"
				currentEntity = nil
				currentStatus = nil
			}

		case "SBR":
			if currentSubscriber != nil {
				currentSubscriber.SBR = seg
			}

		case "NM1":
			currentEntity = &Loop277Entity{NM1: seg}
			switch state {
			case "2000A":
				if currentSource != nil {
					currentSource.SourceInfo = currentEntity
					state = "2100A"
				}
			case "2000B":
				if currentReceiver != nil {
					currentReceiver.ReceiverInfo = currentEntity
					state = "2100B"
				}
			case "2000C":
				if currentSubscriber != nil {
					currentSubscriber.SubscriberInfo = currentEntity
					state = "2100C"
				}
			case "2000D":
				if currentDependent != nil {
					currentDependent.DependentInfo = currentEntity
					state = "2100D"
				}
			}

		case "N3":
			if currentEntity != nil {
				currentEntity.N3 = seg
			}

		case "N4":
			if currentEntity != nil {
				currentEntity.N4 = seg
			}

		case "PER":
			if currentEntity != nil {
				currentEntity.PER = append(currentEntity.PER, seg)
			}

		case "DMG":
			if currentEntity != nil {
				currentEntity.DMG = seg
			}

		case "TRN":
			// TRN starts a new claim status block (Loop 2200)
			currentStatus = &Loop277Status{TRN: seg}
			if state == "2100D" || state == "2200D" {
				if currentDependent != nil {
					currentDependent.ClaimStatuses = append(currentDependent.ClaimStatuses, currentStatus)
					state = "2200D"
				}
			} else if currentSubscriber != nil {
				currentSubscriber.ClaimStatuses = append(currentSubscriber.ClaimStatuses, currentStatus)
				state = "2200C"
			}
			currentServiceLine = nil

		case "STC":
			// STC can appear at claim level or service line level
			if currentServiceLine != nil {
				currentServiceLine.STC = append(currentServiceLine.STC, seg)
			} else if currentStatus != nil {
				currentStatus.STC = append(currentStatus.STC, seg)
			}

		case "REF":
			if currentServiceLine != nil {
				currentServiceLine.REF = append(currentServiceLine.REF, seg)
			} else if currentStatus != nil {
				currentStatus.REF = append(currentStatus.REF, seg)
			} else if currentEntity != nil {
				currentEntity.REF = append(currentEntity.REF, seg)
			}

		case "DTP":
			if currentServiceLine != nil {
				currentServiceLine.DTP = append(currentServiceLine.DTP, seg)
			} else if currentStatus != nil {
				currentStatus.DTP = append(currentStatus.DTP, seg)
			}

		case "QTY":
			if currentStatus != nil {
				currentStatus.QTY = append(currentStatus.QTY, seg)
			}

		case "AMT":
			if currentStatus != nil {
				currentStatus.AMT = append(currentStatus.AMT, seg)
			}

		case "SVC":
			// SVC starts a service line status block (Loop 2220)
			currentServiceLine = &Loop277ServiceLineStatus{SVC: seg}
			if currentStatus != nil {
				currentStatus.ServiceLines = append(currentStatus.ServiceLines, currentServiceLine)
			}
		}
	}

	return result
}
