package console_setting

import "testing"

func TestMatchErrorCaptureRules(t *testing.T) {
	rules := []ErrorCaptureRule{
		{Id: "a", Keyword: "Rate limit", Enabled: true, MaxRecords: 100},
		{Id: "b", Keyword: "insufficient_quota", Enabled: true, MaxRecords: 50},
		{Id: "c", Keyword: "timeout", Enabled: false, MaxRecords: 100}, // disabled
		{Id: "d", Keyword: "", Enabled: true, MaxRecords: 100},          // empty -> skip
	}

	got := MatchErrorCaptureRules("Error: RATE LIMIT exceeded", rules) // 大小写不敏感
	if len(got) != 1 || got[0].Id != "a" {
		t.Fatalf("expected rule a, got %+v", got)
	}

	if m := MatchErrorCaptureRules("upstream timeout occurred", rules); len(m) != 0 {
		t.Fatalf("disabled rule must not match, got %+v", m)
	}

	if m := MatchErrorCaptureRules("anything", rules); len(m) != 0 {
		t.Fatalf("empty keyword must not match, got %+v", m)
	}
}

func TestNormalizeRulesJSON(t *testing.T) {
	n := 0
	genID := func() string { n++; return "gen" }
	in := `[
		{"keyword":"  spaces  ","label":"x","enabled":true,"max_records":0},
		{"id":"keep","keyword":"k","enabled":true,"max_records":99999},
		{"keyword":"   ","enabled":true,"max_records":10}
	]`
	out, err := NormalizeRulesJSON(in, genID)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	rules, err := parseRules(out)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(rules) != 2 { // 空白关键词那条被丢弃
		t.Fatalf("expected 2 rules, got %d: %+v", len(rules), rules)
	}
	if rules[0].Id != "gen" || rules[0].Keyword != "spaces" || rules[0].MaxRecords != 100 {
		t.Fatalf("rule0 not normalized: %+v", rules[0])
	}
	if rules[1].Id != "keep" || rules[1].MaxRecords != 1000 { // 夹紧到上限
		t.Fatalf("rule1 not clamped: %+v", rules[1])
	}
	if n != 1 {
		t.Fatalf("genID expected called once, got %d", n)
	}
}
