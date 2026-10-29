package runtime

import (
	"ai-meeting/models"
	mysqlrepo "ai-meeting/repositories/mysql"
	"context"
)

type snapshotRepository interface {
	FindHotSnapshot(ctx context.Context, sessionID string) (*models.InterviewRuntimeHotSnapshot, error)
	ArchiveTurn(ctx context.Context, sessionID, requestID string, turn models.InterviewTurnLog, snapshotVersion int64) (int64, error)
	CompareAndSetHotSnapshot(ctx context.Context, sessionID string, expectedVersion int64, snap *models.InterviewRuntimeHotSnapshot) (bool, error)
	UpsertHotSnapshot(ctx context.Context, snap *models.InterviewRuntimeHotSnapshot) error
	FindColdSnapshot(ctx context.Context, sessionID string) (*models.InterviewRuntimeColdSnapshot, error)
	UpsertColdSnapshot(ctx context.Context, snap *models.InterviewRuntimeColdSnapshot) error
	FindTurnArchives(ctx context.Context, sessionID string) ([]models.InterviewSessionTurnArchive, error)
}

type mysqlSnapshotRepository struct{}

func (mysqlSnapshotRepository) FindHotSnapshot(ctx context.Context, sessionID string) (*models.InterviewRuntimeHotSnapshot, error) {
	return mysqlrepo.FindHotSnapshot(ctx, sessionID)
}

func (mysqlSnapshotRepository) ArchiveTurn(ctx context.Context, sessionID, requestID string, turn models.InterviewTurnLog, snapshotVersion int64) (int64, error) {
	return mysqlrepo.ArchiveTurn(ctx, sessionID, requestID, turn, snapshotVersion)
}

func (mysqlSnapshotRepository) CompareAndSetHotSnapshot(ctx context.Context, sessionID string, expectedVersion int64, snap *models.InterviewRuntimeHotSnapshot) (bool, error) {
	return mysqlrepo.CompareAndSetHotSnapshot(ctx, sessionID, expectedVersion, snap)
}

func (mysqlSnapshotRepository) UpsertHotSnapshot(ctx context.Context, snap *models.InterviewRuntimeHotSnapshot) error {
	return mysqlrepo.UpsertHotSnapshot(ctx, snap)
}

func (mysqlSnapshotRepository) FindColdSnapshot(ctx context.Context, sessionID string) (*models.InterviewRuntimeColdSnapshot, error) {
	return mysqlrepo.FindColdSnapshot(ctx, sessionID)
}

func (mysqlSnapshotRepository) UpsertColdSnapshot(ctx context.Context, snap *models.InterviewRuntimeColdSnapshot) error {
	return mysqlrepo.UpsertColdSnapshot(ctx, snap)
}

func (mysqlSnapshotRepository) FindTurnArchives(ctx context.Context, sessionID string) ([]models.InterviewSessionTurnArchive, error) {
	return mysqlrepo.FindTurnArchives(ctx, sessionID)
}
