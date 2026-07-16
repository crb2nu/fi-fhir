package session

import "testing"

func TestBuildHL7v2LineageUsesInspectorCompatibleRepeatedPaths(t *testing.T) {
	raw := "MSH|^~\\&|LAB|FAC|FHIR|FAC|20260716120000||ORU^R01|control-1|P|2.5.1\r" +
		"PID|1||mrn-1^^^FAC^MR||Patient^Test||19800101|F\r" +
		"OBR|1|||panel^Panel\r" +
		"OBX|1|NM|first^First||7.1|mg/dL\r" +
		"OBX|2|ST|second^Second||sensitive result"

	links := BuildHL7v2Lineage(raw, nil)
	want := map[string]string{
		"MSH-9":    "event.type",
		"PID-5":    "event.patient.name",
		"OBX[0]-3": "event.results[0].test",
		"OBX[0]-5": "event.results[0].result",
		"OBX[1]-3": "event.results[1].test",
		"OBX[1]-5": "event.results[1].result",
	}

	for _, link := range links {
		if target, ok := want[link.SourcePath]; ok {
			if link.TargetPath != target {
				t.Errorf("lineage %s target = %q, want %q", link.SourcePath, link.TargetPath, target)
			}
			delete(want, link.SourcePath)
		}
	}
	for path, target := range want {
		t.Errorf("missing lineage %s -> %s", path, target)
	}
}
