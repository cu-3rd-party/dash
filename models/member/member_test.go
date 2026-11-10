package member_test

import (
	"testing"

	"git.cu3rd.ru/cu3rd/dash/models/db"
	member_model "git.cu3rd.ru/cu3rd/dash/models/member"
	"git.cu3rd.ru/cu3rd/dash/models/unittest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoles(t *testing.T) {
	ed := &member_model.Member{Role: member_model.RoleEditor}
	assert.True(t, ed.Can(member_model.RoleViewer))
	assert.True(t, ed.Can(member_model.RoleEditor))
	assert.False(t, ed.Can(member_model.RoleOwner))
	var nobody *member_model.Member
	assert.False(t, nobody.Can(member_model.RoleViewer), "templates call Can on a nil doer")
}

func TestNormalizeLogin(t *testing.T) {
	login, err := member_model.NormalizeLogin("  @Fedya.Z ")
	require.NoError(t, err)
	assert.Equal(t, "fedya.z", login)
	_, err = member_model.NormalizeLogin("bad login")
	require.ErrorIs(t, err, db.ErrInvalidArgument)
}

func TestUpsert(t *testing.T) {
	unittest.PrepareTestDatabase(t)
	ctx := t.Context()

	require.NoError(t, member_model.Upsert(ctx, &member_model.Member{Login: "vi", Role: member_model.RoleEditor}))
	vi := unittest.AssertExistsAndLoadBean(t, &member_model.Member{Login: "vi"})
	assert.Equal(t, member_model.RoleEditor, vi.Role)

	require.NoError(t, member_model.Upsert(ctx, &member_model.Member{Login: "new", Role: member_model.RoleViewer, Name: "Новый"}))
	unittest.AssertCount(t, new(member_model.Member), 4)

	require.ErrorIs(t, member_model.Upsert(ctx, &member_model.Member{Login: "x", Role: "god"}), db.ErrInvalidArgument)

	owners, err := member_model.CountOwners(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), owners)

	_, err = member_model.GetByLogin(ctx, "missing")
	require.ErrorIs(t, err, db.ErrNotExist)
}

func TestTelegram(t *testing.T) {
	m := &member_model.Member{Login: "ed"}
	assert.Equal(t, "ed", m.TelegramHandle(), "defaults to the Forgejo login")
	m.Telegram = "ed_tg"
	assert.Equal(t, "ed_tg", m.TelegramHandle())

	for in, want := range map[string]string{"@Name_1": "Name_1", "https://t.me/pxc1984": "pxc1984", "": ""} {
		got, err := member_model.NormalizeTelegram(in)
		require.NoError(t, err)
		assert.Equal(t, want, got)
	}
	_, err := member_model.NormalizeTelegram("a b")
	require.ErrorIs(t, err, db.ErrInvalidArgument)
}
