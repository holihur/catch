package api

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestCompareMetric(t *testing.T) {
	cases := []struct {
		value     float64
		operator  string
		threshold float64
		want      bool
	}{
		{11, "gt", 10, true}, {10, "gt", 10, false},
		{10, "gte", 10, true}, {9, "gte", 10, false},
		{9, "lt", 10, true}, {10, "lt", 10, false},
		{10, "lte", 10, true}, {11, "lte", 10, false},
		{10, "unknown", 10, false},
	}
	for _, c := range cases {
		if got := compareMetric(c.value, c.operator, c.threshold); got != c.want {
			t.Fatalf("compareMetric(%v, %q, %v) = %v, want %v", c.value, c.operator, c.threshold, got, c.want)
		}
	}
}

func TestBuildAlertRuleValidation(t *testing.T) {
	tenant := primitive.NewObjectID()
	project := primitive.NewObjectID()
	threshold := 10.0
	rule, msg := buildAlertRule(tenant, project, "延迟", "http.latency", "", "avg", "gt", &threshold, 300, "warning", nil, nil)
	if msg != "" || rule == nil || rule.Aggregation != "avg" || !rule.Enabled {
		t.Fatalf("valid rule rejected: %v %v", rule, msg)
	}
	if _, msg := buildAlertRule(tenant, project, "延迟", "http.latency", "", "sum", "gt", &threshold, 300, "warning", nil, nil); msg != "" {
		// sum is valid as well
		t.Fatalf("sum aggregation rejected: %s", msg)
	}
	if _, msg := buildAlertRule(tenant, project, "延迟", "http.latency", "", "avg", "gt", &threshold, 10, "warning", nil, nil); msg == "" {
		t.Fatal("expected short window to be rejected")
	}
}
