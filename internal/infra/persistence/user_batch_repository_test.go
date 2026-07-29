package persistence_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NoxiouSi/eino-risk-qa/internal/application"
	"github.com/NoxiouSi/eino-risk-qa/internal/infra/persistence"
)

func TestGORMUserBatchRepository_EnsureUser_CreatesThenIsIdempotent(t *testing.T) {
	db := setupTestDB(t)
	repo := persistence.NewGORMUserBatchRepository(db)
	ctx := context.Background()

	require.NoError(t, repo.EnsureUser(ctx, application.User{UserID: "u_1", Name: "张三"}))
	require.NoError(t, repo.EnsureUser(ctx, application.User{UserID: "u_1", Name: "张三(重复调用)"}))

	var count int64
	require.NoError(t, db.Table("users").Where("user_id = ?", "u_1").Count(&count).Error)
	assert.EqualValues(t, 1, count, "重复EnsureUser不应插入第二条记录")
}

func TestGORMUserBatchRepository_CreateAndFindBatch(t *testing.T) {
	db := setupTestDB(t)
	repo := persistence.NewGORMUserBatchRepository(db)
	ctx := context.Background()

	require.NoError(t, repo.EnsureUser(ctx, application.User{UserID: "u_2", Name: "李四"}))
	require.NoError(t, repo.CreateBatch(ctx, application.Batch{BatchID: "batch_x", UserID: "u_2"}))

	b, err := repo.FindBatch(ctx, "batch_x")
	require.NoError(t, err)
	assert.Equal(t, "u_2", b.UserID)
	assert.False(t, b.CreatedAt.IsZero())
}

func TestGORMUserBatchRepository_FindBatch_NotFound(t *testing.T) {
	db := setupTestDB(t)
	repo := persistence.NewGORMUserBatchRepository(db)

	_, err := repo.FindBatch(context.Background(), "batch_missing")

	assert.ErrorIs(t, err, application.ErrBatchNotFound)
}
