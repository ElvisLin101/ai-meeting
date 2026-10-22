package agentctx

import (
	"fmt"
	"strings"
)

// ProfileQuestion 是需要用户确认的模糊/冲突项。
type ProfileQuestion struct {
	Field    string `json:"field"`
	Key      string `json:"key"`
	Value    string `json:"value"`
	Question string `json:"question"`
}

// ProfileDiff 是 L2 consolidation 的增量输出。
type ProfileDiff struct {
	Reason     string            `json:"reason"`
	Add        []ProfileFact     `json:"add,omitempty"`
	Update     []ProfileFact     `json:"update,omitempty"`
	Supersede  []string          `json:"supersede,omitempty"`
	Reject     []uint            `json:"reject,omitempty"`
	Used       []uint            `json:"used,omitempty"`
	AskConfirm []ProfileQuestion `json:"ask_confirm,omitempty"`
	NoChange   bool              `json:"no_change,omitempty"`
}

// ApplyProfileDiff 把 L2 diff 应用到画像副本。
// 返回新画像、待确认项、是否有实际变更。
//
// 冲突策略:
//   - 新事实置信度明显更高 → 旧事实 superseded；
//   - 置信度接近/新事实更弱 → 不覆盖，生成确认问题；
//   - update 指向不存在的事实 → 忽略。
func ApplyProfileDiff(p *UserProfileData, diff ProfileDiff) (*UserProfileData, []ProfileQuestion, bool, error) {
	next := cloneProfile(p)
	NormalizeProfile(next)

	activeByKey := make(map[string]int)
	idIndex := make(map[string]int)
	for i, f := range next.Facts {
		idIndex[f.ID] = i
		if f.Status == "" || f.Status == "active" {
			activeByKey[factKey(f)] = i
		}
	}

	changed := false
	questions := make([]ProfileQuestion, 0, len(diff.AskConfirm))
	addQuestion := func(f ProfileFact) {
		questions = append(questions, ProfileQuestion{
			Field: f.Field, Key: f.Key, Value: f.Value,
			Question: fmt.Sprintf("关于「%s」，之前记录的是「%s」，现在是「%s」，以哪个为准？",
				displayFactKey(f), currentValue(next, f), f.Value),
		})
	}

	for _, id := range diff.Supersede {
		if idx, ok := idIndex[id]; ok && isActive(next.Facts[idx]) {
			next.Facts[idx].Status = "superseded"
			next.Facts[idx].UpdatedAt = nowString()
			changed = true
		}
	}

	for _, f := range diff.Update {
		idx, ok := idIndex[f.ID]
		if !ok || !isActive(next.Facts[idx]) {
			continue
		}
		old := next.Facts[idx]
		if f.Field != "" && f.Field != old.Field {
			old.Field = f.Field
		}
		if f.Key != "" && f.Key != old.Key {
			old.Key = f.Key
		}
		if f.Value != "" {
			old.Value = f.Value
		}
		if f.Confidence > 0 {
			old.Confidence = f.Confidence
		}
		if f.Quote != "" {
			old.Quote = f.Quote
		}
		if f.Source != "" {
			old.Source = f.Source
		}
		if old.Status == "" {
			old.Status = "active"
		}
		old.UpdatedAt = nowString()
		next.Facts[idx] = old
		activeByKey[factKey(old)] = idx
		changed = true
	}

	for _, f := range diff.Add {
		f.Field = strings.TrimSpace(f.Field)
		f.Value = strings.TrimSpace(f.Value)
		if f.Field == "" || f.Value == "" {
			continue
		}
		if f.Status == "" {
			f.Status = "active"
		}
		if f.Confidence <= 0 {
			f.Confidence = 0.6
		}
		f.UpdatedAt = nowString()
		f.ID = factID(f.Field, f.Key, f.Value)

		if idx, ok := activeByKey[factKey(f)]; ok && isActive(next.Facts[idx]) {
			existing := next.Facts[idx]
			if existing.Value == f.Value {
				// 同一事实：只提升置信度/补证据。
				if f.Confidence > existing.Confidence {
					existing.Confidence = f.Confidence
				}
				if existing.Quote == "" && f.Quote != "" {
					existing.Quote = f.Quote
				}
				existing.UpdatedAt = nowString()
				next.Facts[idx] = existing
				changed = true
				continue
			}
			if f.Confidence >= existing.Confidence+0.05 {
				next.Facts[idx].Status = "superseded"
				next.Facts[idx].UpdatedAt = nowString()
				next.Facts = append(next.Facts, f)
				activeByKey[factKey(f)] = len(next.Facts) - 1
				changed = true
			} else {
				addQuestion(f)
			}
			continue
		}

		next.Facts = append(next.Facts, f)
		activeByKey[factKey(f)] = len(next.Facts) - 1
		changed = true
	}

	for _, q := range diff.AskConfirm {
		questions = append(questions, q)
	}
	if len(questions) > 0 {
		changed = true
	}
	return next, questions, changed, nil
}

func cloneProfile(p *UserProfileData) *UserProfileData {
	if p == nil {
		return &UserProfileData{Version: 1}
	}
	cp := *p
	cp.Facts = append([]ProfileFact(nil), p.Facts...)
	return &cp
}

func isActive(f ProfileFact) bool {
	return f.Status == "" || f.Status == "active"
}

func factKey(f ProfileFact) string {
	return f.Field + "|" + f.Key
}

func currentValue(p *UserProfileData, f ProfileFact) string {
	if p == nil {
		return ""
	}
	for _, item := range p.Facts {
		if isActive(item) && factKey(item) == factKey(f) {
			return item.Value
		}
	}
	return ""
}

func displayFactKey(f ProfileFact) string {
	if f.Key != "" {
		return f.Field + "/" + f.Key
	}
	return f.Field
}
