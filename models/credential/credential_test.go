package credential_test

import (
	"testing"

	credential_model "git.cu3rd.ru/cu3rd/dash/models/credential"
	"git.cu3rd.ru/cu3rd/dash/models/db"
	"git.cu3rd.ru/cu3rd/dash/models/unittest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveAndGet(t *testing.T) {
	unittest.PrepareTestDatabase(t)
	ctx := t.Context()

	_, err := credential_model.Get(ctx, "cloudtips")
	require.ErrorIs(t, err, db.ErrNotExist)

	require.NoError(t, credential_model.Save(ctx, "cloudtips", "a", "env-1"))
	require.NoError(t, credential_model.Save(ctx, "cloudtips", "b", "env-2"))
	require.NoError(t, credential_model.Save(ctx, "boosty", "x", ""))

	c, err := credential_model.Get(ctx, "cloudtips")
	require.NoError(t, err)
	assert.Equal(t, "b", c.Value)
	assert.Equal(t, "env-2", c.Seed)
	unittest.AssertCount(t, &credential_model.Credential{}, 2)
}
