package mongo

import "testing"

// ============================================================
// 分页参数归一化测试
// ============================================================

func TestNormalizePage(t *testing.T) {
	tests := []struct {
		name       string
		page, size int
		wantPage   int
		wantSize   int
	}{
		{"正常值", 3, 20, 3, 20},
		{"page 为 0", 0, 20, 1, 20},
		{"page 为负", -5, 20, 1, 20},
		{"size 为 0", 1, 0, 1, 10},
		{"size 为负", 1, -3, 1, 10},
		{"size 超上限", 1, 200, 1, 100},
		{"两者均为 0", 0, 0, 1, 10},
		{"边界 size=100", 1, 100, 1, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, s := normalizePage(tt.page, tt.size)
			if p != tt.wantPage || s != tt.wantSize {
				t.Errorf("normalizePage(%d, %d) = (%d, %d), want (%d, %d)",
					tt.page, tt.size, p, s, tt.wantPage, tt.wantSize)
			}
		})
	}
}
