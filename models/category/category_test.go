package category_test

import (
	"testing"

	category_model "git.cu3rd.ru/cu3rd/dash/models/category"
	"git.cu3rd.ru/cu3rd/dash/models/db"
	"git.cu3rd.ru/cu3rd/dash/models/unittest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListCountsExpenses(t *testing.T) {
	unittest.PrepareTestDatabase(t)

	cats, err := category_model.List(t.Context())
	require.NoError(t, err)
	require.Len(t, cats, 3)
	assert.Equal(t, "servers", cats[0].Slug)
	assert.Equal(t, int64(3), cats[0].ExpenseCount)
	assert.Equal(t, int64(2), cats[1].ExpenseCount)
	assert.Equal(t, int64(0), cats[2].ExpenseCount)
}

func TestCreateUpdateDelete(t *testing.T) {
	unittest.PrepareTestDatabase(t)
	ctx := t.Context()

	require.ErrorIs(t, category_model.Create(ctx, &category_model.Category{Name: "Дубль", Slug: "servers"}), db.ErrInvalidArgument)
	require.ErrorIs(t, category_model.Create(ctx, &category_model.Category{Name: "X", Slug: "Не латиница"}), db.ErrInvalidArgument)

	c := &category_model.Category{Name: "Почта", Slug: " Mail "}
	require.NoError(t, category_model.Create(ctx, c))
	assert.Equal(t, "mail", c.Slug)

	c.Name = "Почта и рассылки"
	require.NoError(t, category_model.Update(ctx, c), "keeping its own slug is fine")

	require.ErrorIs(t, category_model.Delete(ctx, 1), db.ErrInvalidArgument, "category with expenses")
	require.NoError(t, category_model.Delete(ctx, 3))
	unittest.AssertNotExistsBean(t, &category_model.Category{ID: 3})
}

func TestMoveAndAppend(t *testing.T) {
	unittest.PrepareTestDatabase(t)
	ctx := t.Context()
	slugs := func() []string {
		cats, err := category_model.List(ctx)
		require.NoError(t, err)
		out := make([]string, 0, len(cats))
		for _, c := range cats {
			out = append(out, c.Slug)
		}
		return out
	}

	c := &category_model.Category{Name: "Почта", Slug: "mail"}
	require.NoError(t, category_model.Create(ctx, c))
	assert.Equal(t, 40, c.SortOrder, "a new category goes to the end")
	assert.Equal(t, []string{"servers", "domains", "saas", "mail"}, slugs())

	require.NoError(t, category_model.Move(ctx, c.ID, -1))
	assert.Equal(t, []string{"servers", "domains", "mail", "saas"}, slugs())
	require.NoError(t, category_model.Move(ctx, 1, 1))
	assert.Equal(t, []string{"domains", "servers", "mail", "saas"}, slugs())
	require.NoError(t, category_model.Move(ctx, 2, -1), "the first one stays first")
	assert.Equal(t, []string{"domains", "servers", "mail", "saas"}, slugs())
	require.ErrorIs(t, category_model.Move(ctx, 999, 1), db.ErrNotExist)

	// saving the name and description does not touch the position
	d := unittest.AssertExistsAndLoadBean(t, &category_model.Category{Slug: "domains"})
	d.Name = "Доменные имена"
	require.NoError(t, category_model.Update(ctx, d))
	assert.Equal(t, []string{"domains", "servers", "mail", "saas"}, slugs())
}
