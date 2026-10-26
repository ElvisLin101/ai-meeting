package agentctx

import (
	"context"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"

	"ai-meeting/pkg/singleflight"
	"ai-meeting/repositories"
	mysqlrepo "ai-meeting/repositories/mysql"
)

func withSQLMock(t *testing.T) sqlmock.Sqlmock {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	gormDB, err := gorm.Open(gormmysql.New(gormmysql.Config{
		Conn: sqlDB, SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm sqlmock: %v", err)
	}
	old := mysqlrepo.DB
	mysqlrepo.DB = gormDB
	t.Cleanup(func() {
		mysqlrepo.DB = old
		_ = sqlDB.Close()
	})
	return mock
}

func TestCompressAdvancesBoundaryAndRebuildsRecent(t *testing.T) {
	withTestRedis(t)
	mock := withSQLMock(t)
	ctx := context.Background()
	sessionID, userID := "s1", "u1"

	if err := repositories.SetCompressedSummary(ctx, sessionID, "old-summary", 1); err != nil {
		t.Fatalf("seed summary: %v", err)
	}

	rows := sqlmock.NewRows([]string{
		"id", "session_id", "user_id", "role", "content", "sequence", "metadata", "created_at",
	})
	// 倒序：最新在前。seq 2~5 未被摘要覆盖。
	for seq := int64(5); seq >= 2; seq-- {
		rows.AddRow(seq, sessionID, userID, "user", strings.Repeat("中", 200), seq, "", time.Now())
	}
	mock.ExpectQuery("SELECT \\* FROM `messages` WHERE session_id = \\? AND user_id = \\? ORDER BY sequence DESC").
		WithArgs(sessionID, userID).
		WillReturnRows(rows)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `compressed_contexts`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	orig := callCompressAI
	callCompressAI = func(ctx context.Context, writer *singleflight.StreamWriter, prevSummary, newText string) (string, error) {
		if !strings.Contains(prevSummary, "old-summary") {
			t.Errorf("previous summary should be passed to compressor, got %q", prevSummary)
		}
		return "new-summary", nil
	}
	t.Cleanup(func() { callCompressAI = orig })

	if err := doCompress(ctx, nil, sessionID, userID, 10); err != nil {
		t.Fatalf("compress: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}

	summary, covered, ok := repositories.GetCompressedSummary(ctx, sessionID)
	if !ok || summary != "new-summary" || covered != 3 {
		t.Fatalf("unexpected summary: summary=%q covered=%d ok=%v", summary, covered, ok)
	}
	recent, err := repositories.GetRecentMessages(ctx, sessionID)
	if err != nil {
		t.Fatalf("read recent: %v", err)
	}
	if len(recent) != 2 || recent[0].Sequence != 4 || recent[1].Sequence != 5 {
		t.Fatalf("recent should be tail seq 4,5: %+v", recent)
	}
}
