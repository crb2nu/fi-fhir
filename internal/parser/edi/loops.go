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
