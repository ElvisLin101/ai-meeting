package user

import (
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

// ============================================================
// 用户服务密码逻辑测试: bcrypt 哈希 + 旧明文兼容迁移
// (Login/Register 依赖 mysqlrepo 全局 DB, 不在单测范围)
// ============================================================

func TestHashPassword(t *testing.T) {
	hash, err := hashPassword("my-pass-123")
	if err != nil {
		t.Fatalf("hashPassword failed: %v", err)
	}
	if !strings.HasPrefix(hash, "$2") {
		t.Errorf("哈希应带 bcrypt 前缀, got %q", hash)
	}
	if hash == "my-pass-123" {
		t.Error("哈希不应等于明文")
	}

	// 不同明文产生不同哈希
	hash2, _ := hashPassword("my-pass-123")
	hash3, _ := hashPassword("other-pass")
	if hash == hash2 {
		t.Error("同一明文两次哈希应不同(bcrypt 加盐)")
	}
	if hash2 == hash3 {
		t.Error("不同明文哈希不应相同")
	}
}

func TestVerifyPassword(t *testing.T) {
	hash, _ := hashPassword("secret")

	t.Run("bcrypt 匹配", func(t *testing.T) {
		matched, migrate, err := verifyPassword(hash, "secret")
		if err != nil || !matched || migrate {
			t.Errorf("matched=%v migrate=%v err=%v, want true/false/nil", matched, migrate, err)
		}
	})

	t.Run("bcrypt 不匹配", func(t *testing.T) {
		matched, migrate, err := verifyPassword(hash, "wrong")
		if err != nil || matched || migrate {
			t.Errorf("matched=%v migrate=%v err=%v, want false/false/nil", matched, migrate, err)
		}
	})

	t.Run("旧明文匹配需迁移", func(t *testing.T) {
		matched, migrate, err := verifyPassword("plain-old-pass", "plain-old-pass")
		if err != nil || !matched || !migrate {
			t.Errorf("matched=%v migrate=%v err=%v, want true/true/nil", matched, migrate, err)
		}
	})

	t.Run("旧明文不匹配", func(t *testing.T) {
		matched, migrate, err := verifyPassword("plain-old-pass", "other")
		if err != nil || matched || !migrate {
			t.Errorf("matched=%v migrate=%v err=%v, want false/true/nil", matched, migrate, err)
		}
	})
}

func TestVerifyPassword_RealBcryptHash(t *testing.T) {
	// 用标准库生成的真实 bcrypt 哈希验证兼容性
	real, err := bcrypt.GenerateFromPassword([]byte("real-pass"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("bcrypt failed: %v", err)
	}
	matched, migrate, err := verifyPassword(string(real), "real-pass")
	if err != nil || !matched || migrate {
		t.Errorf("真实 bcrypt 哈希校验失败: matched=%v migrate=%v err=%v", matched, migrate, err)
	}
}

func TestGetUserService_Singleton(t *testing.T) {
	a := GetUserService()
	b := GetUserService()
	if a == nil || a != b {
		t.Error("GetUserService 应返回同一单例")
	}
}
