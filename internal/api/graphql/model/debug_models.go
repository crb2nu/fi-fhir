package model

import "time"

// DebugSessionModel represents a debug session in the GraphQL API.
type DebugSessionModel struct {
	ID          string             `json:"id"`
	WorkflowID  string             `json:"workflowId"`
	State       string             `json:"state"`
	Breakpoints []*BreakpointModel `json:"breakpoints"`
	Steps       []*DebugStepModel  `json:"steps"`
	CreatedAt   time.Time          `json:"createdAt"`
}

// BreakpointModel represents a breakpoint in the GraphQL API.
type BreakpointModel struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

// DebugStepModel represents a debug step in the GraphQL API.
type DebugStepModel struct {
	StepNumber int                    `json:"stepNumber"`
	Kind       string                 `json:"kind"`
	Name       string                 `json:"name"`
	Variables  map[string]interface{} `json:"variables"`
	Timestamp  time.Time              `json:"timestamp"`
	SpanName   string                 `json:"spanName"`
}

// ParseEventModel represents a parse event in the GraphQL API.
type ParseEventModel struct {
	SegmentIndex int                    `json:"segmentIndex"`
	SegmentType  string                 `json:"segmentType"`
	RawSegment   string                 `json:"rawSegment"`
	Fields       map[string]interface{} `json:"fields"`
	Warnings     []string               `json:"warnings"`
	IsComplete   bool                   `json:"isComplete"`
}

// TraceSpanModel represents a trace span in the GraphQL API.
type TraceSpanModel struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	ParentID   *string                `json:"parentId"`
	StartTime  time.Time              `json:"startTime"`
	EndTime    *time.Time             `json:"endTime"`
	Status     string                 `json:"status"`
	Attributes map[string]interface{} `json:"attributes"`
	Events     []*TraceSpanEventModel `json:"events"`
}

// TraceSpanEventModel represents an event within a trace span.
type TraceSpanEventModel struct {
	Name       string                 `json:"name"`
	Timestamp  time.Time              `json:"timestamp"`
	Attributes map[string]interface{} `json:"attributes"`
}

// StartDebugSessionInput is the input for starting a debug session.
type StartDebugSessionInput struct {
	WorkflowYaml string                 `json:"workflowYaml"`
	Event        map[string]interface{} `json:"event"`
}

// SetBreakpointInput is the input for setting a breakpoint.
type SetBreakpointInput struct {
	SessionID string `json:"sessionId"`
	Type      string `json:"type"`
	Name      string `json:"name"`
}

// LiveParseInput is the input for live parse streaming.
type LiveParseInput struct {
	Message string       `json:"message"`
	Format  SourceFormat `json:"format"`
}
