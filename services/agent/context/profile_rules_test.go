package agentctx

import "testing"

func TestExtractRuleCandidates(t *testing.T) {
	cases := []struct {
		name   string
		query  string
		fields []string
	}{
		{"goal", "我想找后端开发的工作", []string{"goal"}},
		{"constraint", "我只考虑上海，不接受长期加班", []string{"constraint"}},
		{"background", "我有三年 Go 后端经验", []string{"background"}},
		{"preference", "以后回答简短点，用中文", []string{"preference"}},
		{"correction", "我说错了，不是这个意思", []string{"correction"}},
		{"explicit memory", "记住我下周要面试", []string{"explicit_memory"}},
		{"interview", "我面过字节二面，挂了", []string{"interview_state"}},
		{"greeting", "你好", nil},
		{"weather", "今天天气怎么样？", nil},
	}
	for _, tc := range cases {
		got := ExtractRuleCandidates(tc.query)
		fields := make(map[string]bool, len(got))
		for _, c := range got {
			fields[c.Field] = true
			if c.Quote == "" || c.Confidence <= 0 {
				t.Fatalf("%s: candidate missing quote/confidence: %+v", tc.name, c)
			}
		}
		if len(fields) != len(tc.fields) {
			t.Fatalf("%s: got fields %v, want %v", tc.name, fields, tc.fields)
		}
		for _, want := range tc.fields {
			if !fields[want] {
				t.Fatalf("%s: missing field %q in %v", tc.name, want, fields)
			}
		}
	}
}

func TestCandidateHashStableAndDistinct(t *testing.T) {
	a := candidateHash("u1", 10, "goal", "我想找后端")
	b := candidateHash("u1", 10, "goal", "我想找后端")
	c := candidateHash("u2", 10, "goal", "我想找后端")
	if a != b {
		t.Fatal("same candidate should produce the same hash")
	}
	if a == c {
		t.Fatal("different users should produce different hashes")
	}
}
