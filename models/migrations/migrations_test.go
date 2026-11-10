package migrations

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
	"xorm.io/xorm"
)

func TestMigrateIsIdempotent(t *testing.T) {
	x, err := xorm.NewEngine("sqlite", "file:"+filepath.Join(t.TempDir(), "m.db"))
	require.NoError(t, err)
	defer x.Close()

	require.NoError(t, Migrate(t.Context(), x))
	require.NoError(t, Migrate(t.Context(), x))

	v := &Version{}
	has, err := x.ID(1).Get(v)
	require.NoError(t, err)
	assert.True(t, has)
	assert.Equal(t, ExpectedVersion(), v.Version)
}

func TestV3MovesCloudTipsToken(t *testing.T) {
	x, err := xorm.NewEngine("sqlite", "file:"+filepath.Join(t.TempDir(), "m.db"))
	require.NoError(t, err)
	defer x.Close()
	require.NoError(t, Migrate(t.Context(), x))

	// what the first version of the CloudTips import left in the site settings
	_, err = x.Exec("INSERT INTO site_setting (name, value) VALUES ('cloudtips_refresh', 'tok'), ('cloudtips_seed', 'env')")
	require.NoError(t, err)
	require.NoError(t, v3CreateCredential(x))

	rows, err := x.QueryString("SELECT name, value, seed FROM credential")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, map[string]string{"name": "cloudtips", "value": "tok", "seed": "env"}, rows[0])
	left, err := x.QueryString("SELECT name FROM site_setting WHERE name LIKE 'cloudtips%'")
	require.NoError(t, err)
	assert.Empty(t, left)
}
