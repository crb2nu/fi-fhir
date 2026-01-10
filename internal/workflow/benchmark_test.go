package workflow

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// =============================================================================
// Engine Benchmarks
// =============================================================================

// BenchmarkEngineProcess measures event processing throughput.
func BenchmarkEngineProcess(b *testing.B) {
	workflow := &Workflow{
		Name: "benchmark",
		Routes: []Route{
			{
				Name:   "patient_route",
				Filter: Filter{EventType: StringOrSlice{"patient_admit"}},
				Actions: []Action{
					{Type: "log", Config: map[string]string{"level": "info"}},
				},
			},
		},
	}

	engine, err := NewEngine(workflow)
	if err != nil {
		b.Fatalf("NewEngine failed: %v", err)
	}

	event := map[string]interface{}{
		"type":   "patient_admit",
		"source": "epic_adt",
		"patient": map[string]interface{}{
			"mrn":  "12345",
			"name": "John Doe",
		},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		engine.Process(event)
	}
}

// BenchmarkEngineProcess_MultiRoute tests with multiple routes.
func BenchmarkEngineProcess_MultiRoute(b *testing.B) {
	routeCounts := []int{1, 5, 10, 25, 50}

	for _, routeCount := range routeCounts {
		b.Run(fmt.Sprintf("routes=%d", routeCount), func(b *testing.B) {
			routes := make([]Route, routeCount)
			for i := 0; i < routeCount; i++ {
				routes[i] = Route{
					Name:   fmt.Sprintf("route_%d", i),
					Filter: Filter{EventType: StringOrSlice{fmt.Sprintf("event_type_%d", i)}},
					Actions: []Action{
						{Type: "log", Config: map[string]string{"level": "info"}},
					},
				}
			}
			// Add one matching route at the end
			routes[routeCount-1] = Route{
				Name:   "matching_route",
				Filter: Filter{EventType: StringOrSlice{"patient_admit"}},
				Actions: []Action{
					{Type: "log", Config: map[string]string{"level": "info"}},
				},
			}

			workflow := &Workflow{Name: "benchmark", Routes: routes}
			engine, _ := NewEngine(workflow)

			event := map[string]interface{}{
				"type":   "patient_admit",
				"source": "epic",
			}

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				engine.Process(event)
			}
		})
	}
}

// BenchmarkEngineProcess_MultiAction tests with multiple actions per route.
func BenchmarkEngineProcess_MultiAction(b *testing.B) {
	actionCounts := []int{1, 3, 5, 10}

	for _, actionCount := range actionCounts {
		b.Run(fmt.Sprintf("actions=%d", actionCount), func(b *testing.B) {
			actions := make([]Action, actionCount)
			for i := 0; i < actionCount; i++ {
				actions[i] = Action{
					Type:   "log",
					Config: map[string]string{"level": "info", "message": fmt.Sprintf("action_%d", i)},
				}
			}

			workflow := &Workflow{
				Name: "benchmark",
				Routes: []Route{
					{
						Name:    "test_route",
						Filter:  Filter{EventType: StringOrSlice{"patient_admit"}},
						Actions: actions,
					},
				},
			}

			engine, _ := NewEngine(workflow)
			event := map[string]interface{}{"type": "patient_admit"}

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				engine.Process(event)
			}
		})
	}
}

// BenchmarkEngineProcess_NoMatch tests events that match no routes.
func BenchmarkEngineProcess_NoMatch(b *testing.B) {
	workflow := &Workflow{
		Name: "benchmark",
		Routes: []Route{
			{
				Name:    "patient_route",
				Filter:  Filter{EventType: StringOrSlice{"patient_admit"}},
				Actions: []Action{{Type: "log"}},
			},
		},
	}

	engine, _ := NewEngine(workflow)
	event := map[string]interface{}{"type": "lab_result"} // Won't match

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		engine.Process(event)
	}
}

// BenchmarkEngineDryRun measures dry-run processing.
func BenchmarkEngineDryRun(b *testing.B) {
	workflow := &Workflow{
		Name: "benchmark",
		Routes: []Route{
			{
				Name:    "route1",
				Filter:  Filter{EventType: StringOrSlice{"patient_admit"}},
				Actions: []Action{{Type: "log"}, {Type: "webhook"}},
			},
			{
				Name:    "route2",
				Filter:  Filter{Source: StringOrSlice{"epic"}},
				Actions: []Action{{Type: "log"}},
			},
		},
	}

	engine, _ := NewEngine(workflow)
	event := map[string]interface{}{"type": "patient_admit", "source": "epic"}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		engine.DryRun(event)
	}
}

// =============================================================================
// CEL Evaluation Benchmarks
// =============================================================================

// BenchmarkCELEvaluate_Simple tests simple CEL expressions.
func BenchmarkCELEvaluate_Simple(b *testing.B) {
	evaluator, _ := NewCELEvaluator()
	event := map[string]interface{}{
		"type":   "patient_admit",
		"source": "epic",
	}

	// Warm up the cache
	evaluator.Evaluate(`event.type == "patient_admit"`, event)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		evaluator.Evaluate(`event.type == "patient_admit"`, event)
	}
}

// BenchmarkCELEvaluate_Complex tests complex CEL expressions.
func BenchmarkCELEvaluate_Complex(b *testing.B) {
	evaluator, _ := NewCELEvaluator()
	event := map[string]interface{}{
		"type":   "patient_admit",
		"source": "epic",
		"patient": map[string]interface{}{
			"age":    45,
			"status": "active",
		},
		"encounter": map[string]interface{}{
			"class": "inpatient",
		},
	}

	condition := `event.type == "patient_admit" && event.patient.age > 18 && event.encounter.class == "inpatient"`

	// Warm up the cache
	evaluator.Evaluate(condition, event)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		evaluator.Evaluate(condition, event)
	}
}

// BenchmarkCELEvaluate_CacheMiss tests CEL evaluation without cache (compilation).
func BenchmarkCELEvaluate_CacheMiss(b *testing.B) {
	event := map[string]interface{}{
		"type":   "patient_admit",
		"source": "epic",
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Create new evaluator each time to force cache miss
		evaluator, _ := NewCELEvaluator()
		evaluator.Evaluate(`event.type == "patient_admit"`, event)
	}
}

// BenchmarkCELEvaluate_ConditionComplexity tests varying condition complexity.
func BenchmarkCELEvaluate_ConditionComplexity(b *testing.B) {
	conditions := map[string]string{
		"simple":       `event.type == "patient_admit"`,
		"two_clause":   `event.type == "patient_admit" && event.source == "epic"`,
		"three_clause": `event.type == "patient_admit" && event.source == "epic" && event.patient.age > 18`,
		"has_check":    `has(event.patient) && has(event.patient.mrn)`,
		"in_list":      `event.type in ["patient_admit", "patient_update", "patient_discharge"]`,
	}

	evaluator, _ := NewCELEvaluator()
	event := map[string]interface{}{
		"type":   "patient_admit",
		"source": "epic",
		"patient": map[string]interface{}{
			"mrn": "12345",
			"age": 45,
		},
	}

	// Warm up cache
	for _, cond := range conditions {
		evaluator.Evaluate(cond, event)
	}

	for name, cond := range conditions {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				evaluator.Evaluate(cond, event)
			}
		})
	}
}

// =============================================================================
// Filter Matching Benchmarks
// =============================================================================

// BenchmarkFilterMatch_EventType tests event type filter matching.
func BenchmarkFilterMatch_EventType(b *testing.B) {
	workflow := &Workflow{
		Name: "benchmark",
		Routes: []Route{
			{
				Name:    "route",
				Filter:  Filter{EventType: StringOrSlice{"patient_admit", "patient_update", "patient_discharge"}},
				Actions: []Action{{Type: "log"}},
			},
		},
	}

	engine, _ := NewEngine(workflow)
	event := map[string]interface{}{"type": "patient_admit"}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		engine.Process(event)
	}
}

// BenchmarkFilterMatch_Source tests source filter matching.
func BenchmarkFilterMatch_Source(b *testing.B) {
	workflow := &Workflow{
		Name: "benchmark",
		Routes: []Route{
			{
				Name:    "route",
				Filter:  Filter{Source: StringOrSlice{"epic", "cerner", "allscripts"}},
				Actions: []Action{{Type: "log"}},
			},
		},
	}

	engine, _ := NewEngine(workflow)
	event := map[string]interface{}{"type": "test", "source": "epic"}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		engine.Process(event)
	}
}

// BenchmarkFilterMatch_CELCondition tests CEL condition filter matching.
func BenchmarkFilterMatch_CELCondition(b *testing.B) {
	workflow := &Workflow{
		Name: "benchmark",
		Routes: []Route{
			{
				Name: "route",
				Filter: Filter{
					Condition: `event.patient.age >= 65 && event.encounter.class == "emergency"`,
				},
				Actions: []Action{{Type: "log"}},
			},
		},
	}

	engine, _ := NewEngine(workflow)
	event := map[string]interface{}{
		"type": "patient_admit",
		"patient": map[string]interface{}{
			"age": 70,
		},
		"encounter": map[string]interface{}{
			"class": "emergency",
		},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		engine.Process(event)
	}
}

// BenchmarkFilterMatch_Combined tests combined filter matching.
func BenchmarkFilterMatch_Combined(b *testing.B) {
	workflow := &Workflow{
		Name: "benchmark",
		Routes: []Route{
			{
				Name: "route",
				Filter: Filter{
					EventType: StringOrSlice{"patient_admit"},
					Source:    StringOrSlice{"epic"},
					Condition: `event.patient.age >= 18`,
				},
				Actions: []Action{{Type: "log"}},
			},
		},
	}

	engine, _ := NewEngine(workflow)
	event := map[string]interface{}{
		"type":   "patient_admit",
		"source": "epic",
		"patient": map[string]interface{}{
			"age": 45,
		},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		engine.Process(event)
	}
}

// =============================================================================
// Transform Benchmarks
// =============================================================================

// BenchmarkTransform_SetField tests set_field transform performance.
func BenchmarkTransform_SetField(b *testing.B) {
	transformer := NewTransformer(nil)
	event := map[string]interface{}{
		"type":   "patient_admit",
		"source": "epic",
	}

	transform := Transform{
		SetField: `processed_at = "2024-01-15T10:00:00Z"`,
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		transformer.Apply(event, transform)
	}
}

// BenchmarkTransform_Redact tests redact transform performance.
func BenchmarkTransform_Redact(b *testing.B) {
	transformer := NewTransformer(nil)
	event := map[string]interface{}{
		"type": "patient_admit",
		"patient": map[string]interface{}{
			"ssn":  "123-45-6789",
			"name": "John Doe",
		},
	}

	transform := Transform{
		Redact: &RedactConfig{
			Fields: []string{"patient.ssn"},
		},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		transformer.Apply(event, transform)
	}
}

// BenchmarkTransform_Pipeline tests multi-transform pipeline.
func BenchmarkTransform_Pipeline(b *testing.B) {
	transformer := NewTransformer(nil)

	transforms := []Transform{
		{SetField: `processed = "true"`},
		{SetField: `timestamp = "2024-01-15"`},
		{Redact: &RedactConfig{Fields: []string{"patient.ssn"}}},
	}

	event := map[string]interface{}{
		"type": "patient_admit",
		"patient": map[string]interface{}{
			"ssn":  "123-45-6789",
			"name": "John Doe",
		},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var result interface{} = event
		for _, t := range transforms {
			result, _ = transformer.Apply(result, t)
		}
	}
}

// =============================================================================
// Event Type Extraction Benchmarks
// =============================================================================

// BenchmarkGetEventType_Map tests event type extraction from map.
func BenchmarkGetEventType_Map(b *testing.B) {
	engine, _ := NewEngine(&Workflow{Name: "test"})
	event := map[string]interface{}{
		"type":   "patient_admit",
		"source": "epic",
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		engine.getEventType(event)
	}
}

// =============================================================================
// Recording/Replay Benchmarks
// =============================================================================

// BenchmarkMemoryRecorder_Record tests recording performance.
func BenchmarkMemoryRecorder_Record(b *testing.B) {
	recorder := NewMemoryRecorder()
	event := map[string]interface{}{
		"type":   "patient_admit",
		"source": "epic",
	}

	result := &Result{
		RouteResults: []RouteResult{
			{RouteName: "route1", Matched: true, ActionsRun: 2},
		},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		recorder.Record(event, result)
	}
}

// BenchmarkRecordingEngine_Process tests recording engine overhead.
func BenchmarkRecordingEngine_Process(b *testing.B) {
	workflow := &Workflow{
		Name: "benchmark",
		Routes: []Route{
			{
				Name:    "route",
				Filter:  Filter{EventType: StringOrSlice{"patient_admit"}},
				Actions: []Action{{Type: "log"}},
			},
		},
	}

	// Baseline: regular engine
	b.Run("baseline", func(b *testing.B) {
		engine, _ := NewEngine(workflow)
		event := map[string]interface{}{"type": "patient_admit"}

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			engine.Process(event)
		}
	})

	// With recording
	b.Run("with_recording", func(b *testing.B) {
		recorder := NewMemoryRecorder()
		engine, _ := NewRecordingEngine(workflow, recorder)
		event := map[string]interface{}{"type": "patient_admit"}

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			engine.Process(event)
		}
	})
}

// =============================================================================
// Simulation Benchmarks
// =============================================================================

// BenchmarkSimulationEngine_Process tests simulation engine performance.
func BenchmarkSimulationEngine_Process(b *testing.B) {
	workflow := &Workflow{
		Name: "benchmark",
		Routes: []Route{
			{
				Name:    "route",
				Filter:  Filter{EventType: StringOrSlice{"patient_admit"}},
				Actions: []Action{{Type: "log"}, {Type: "webhook"}},
			},
		},
	}

	engine, _ := NewSimulationEngine(workflow)
	event := map[string]interface{}{"type": "patient_admit"}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		engine.Process(event)
	}
}

// =============================================================================
// Concurrency Benchmarks
// =============================================================================

// BenchmarkEngineProcess_Parallel tests concurrent processing.
func BenchmarkEngineProcess_Parallel(b *testing.B) {
	workflow := &Workflow{
		Name: "benchmark",
		Routes: []Route{
			{
				Name:    "route",
				Filter:  Filter{EventType: StringOrSlice{"patient_admit"}},
				Actions: []Action{{Type: "log"}},
			},
		},
	}

	engine, _ := NewEngine(workflow)
	event := map[string]interface{}{"type": "patient_admit", "source": "epic"}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			engine.Process(event)
		}
	})
}

// BenchmarkCELEvaluate_Parallel tests concurrent CEL evaluation.
func BenchmarkCELEvaluate_Parallel(b *testing.B) {
	evaluator, _ := NewCELEvaluator()
	event := map[string]interface{}{
		"type":   "patient_admit",
		"source": "epic",
		"patient": map[string]interface{}{
			"age": 45,
		},
	}

	condition := `event.type == "patient_admit" && event.patient.age > 18`

	// Warm up cache
	evaluator.Evaluate(condition, event)

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			evaluator.Evaluate(condition, event)
		}
	})
}

// =============================================================================
// Memory Benchmarks
// =============================================================================

// BenchmarkEngineCreate measures engine creation overhead.
func BenchmarkEngineCreate(b *testing.B) {
	workflow := &Workflow{
		Name: "benchmark",
		Routes: []Route{
			{
				Name:    "route",
				Filter:  Filter{EventType: StringOrSlice{"patient_admit"}},
				Actions: []Action{{Type: "log"}},
			},
		},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		NewEngine(workflow)
	}
}

// BenchmarkWorkflowParse measures workflow parsing overhead.
func BenchmarkWorkflowParse(b *testing.B) {
	yaml := []byte(`
workflow:
  name: benchmark
  version: "1.0"
  routes:
    - name: patient_route
      filter:
        event_type: patient_admit
        source: epic
        condition: "event.patient.age >= 18"
      transforms:
        - set_field: processed
          value: "true"
      actions:
        - type: log
          level: info
        - type: webhook
          url: "https://example.com/webhook"
`)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ParseWorkflow(yaml)
	}
}

// =============================================================================
// Integration Benchmarks (Full Pipeline)
// =============================================================================

// BenchmarkFullPipeline tests complete event processing pipeline.
func BenchmarkFullPipeline(b *testing.B) {
	workflow := &Workflow{
		Name: "full_pipeline",
		Routes: []Route{
			{
				Name: "patient_route",
				Filter: Filter{
					EventType: StringOrSlice{"patient_admit"},
					Source:    StringOrSlice{"epic", "cerner"},
					Condition: `event.patient.age >= 18`,
				},
				Transforms: []Transform{
					{SetField: `processed = "true"`},
					{SetField: `processed_at = "2024-01-15T10:00:00Z"`},
				},
				Actions: []Action{
					{Type: "log", Config: map[string]string{"level": "info"}},
				},
			},
			{
				Name: "emergency_route",
				Filter: Filter{
					Condition: `event.encounter.class == "emergency"`,
				},
				Actions: []Action{
					{Type: "log", Config: map[string]string{"level": "warn"}},
				},
			},
		},
	}

	engine, _ := NewEngine(workflow)
	event := map[string]interface{}{
		"type":   "patient_admit",
		"source": "epic",
		"patient": map[string]interface{}{
			"mrn":  "12345",
			"name": "John Doe",
			"age":  45,
		},
		"encounter": map[string]interface{}{
			"class":    "emergency",
			"location": "ED-01",
		},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		engine.Process(event)
	}
}

// BenchmarkFullPipeline_WithMetrics tests pipeline with metrics collection.
func BenchmarkFullPipeline_WithMetrics(b *testing.B) {
	workflow := &Workflow{
		Name: "benchmark",
		Routes: []Route{
			{
				Name:    "route",
				Filter:  Filter{EventType: StringOrSlice{"patient_admit"}},
				Actions: []Action{{Type: "log"}},
			},
		},
	}

	b.Run("no_metrics", func(b *testing.B) {
		engine, _ := NewEngine(workflow)
		event := map[string]interface{}{"type": "patient_admit"}

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			engine.Process(event)
		}
	})

	b.Run("with_inmemory_metrics", func(b *testing.B) {
		engine, _ := NewEngine(workflow)
		engine.SetMetrics(NewInMemoryMetrics())
		event := map[string]interface{}{"type": "patient_admit"}

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			engine.Process(event)
		}
	})
}

// BenchmarkFullPipeline_WithTracing tests pipeline with tracing.
func BenchmarkFullPipeline_WithTracing(b *testing.B) {
	workflow := &Workflow{
		Name: "benchmark",
		Routes: []Route{
			{
				Name:    "route",
				Filter:  Filter{EventType: StringOrSlice{"patient_admit"}},
				Actions: []Action{{Type: "log"}},
			},
		},
	}

	b.Run("no_tracing", func(b *testing.B) {
		engine, _ := NewEngine(workflow)
		event := map[string]interface{}{"type": "patient_admit"}

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			engine.Process(event)
		}
	})

	b.Run("with_noop_tracer", func(b *testing.B) {
		engine, _ := NewEngine(workflow)
		engine.SetTracer(&NoOpTracer{})
		event := map[string]interface{}{"type": "patient_admit"}

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			engine.ProcessWithContext(context.Background(), event)
		}
	})
}

// =============================================================================
// Throughput Benchmarks (Messages per Second)
// =============================================================================

// BenchmarkThroughput_Simple provides a baseline throughput measure.
func BenchmarkThroughput_Simple(b *testing.B) {
	workflow := &Workflow{
		Name: "throughput",
		Routes: []Route{
			{
				Name:    "route",
				Filter:  Filter{EventType: StringOrSlice{"test"}},
				Actions: []Action{{Type: "log"}},
			},
		},
	}

	engine, _ := NewEngine(workflow)
	event := map[string]interface{}{"type": "test"}

	start := time.Now()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		engine.Process(event)
	}

	b.StopTimer()
	elapsed := time.Since(start)
	throughput := float64(b.N) / elapsed.Seconds()
	b.ReportMetric(throughput, "events/sec")
}

// BenchmarkThroughput_Complex provides a complex workflow throughput measure.
func BenchmarkThroughput_Complex(b *testing.B) {
	workflow := &Workflow{
		Name: "throughput_complex",
		Routes: []Route{
			{
				Name: "route1",
				Filter: Filter{
					EventType: StringOrSlice{"patient_admit"},
					Condition: `event.patient.age >= 18`,
				},
				Transforms: []Transform{
					{SetField: `processed = "true"`},
				},
				Actions: []Action{{Type: "log"}, {Type: "log"}},
			},
			{
				Name: "route2",
				Filter: Filter{
					EventType: StringOrSlice{"patient_admit"},
					Condition: `event.patient.age >= 65`,
				},
				Actions: []Action{{Type: "log"}},
			},
		},
	}

	engine, _ := NewEngine(workflow)
	event := map[string]interface{}{
		"type": "patient_admit",
		"patient": map[string]interface{}{
			"age": 70,
		},
	}

	start := time.Now()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		engine.Process(event)
	}

	b.StopTimer()
	elapsed := time.Since(start)
	throughput := float64(b.N) / elapsed.Seconds()
	b.ReportMetric(throughput, "events/sec")
}
