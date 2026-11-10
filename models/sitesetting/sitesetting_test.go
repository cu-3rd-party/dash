package sitesetting_test

import (
	"testing"

	"git.cu3rd.ru/cu3rd/dash/models/db"
	sitesetting_model "git.cu3rd.ru/cu3rd/dash/models/sitesetting"
	"git.cu3rd.ru/cu3rd/dash/models/unittest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetAndRates(t *testing.T) {
	unittest.PrepareTestDatabase(t)
	ctx := t.Context()

	rates, err := sitesetting_model.Rates(ctx)
	require.NoError(t, err)
	assert.InDelta(t, 90.0, rates["USD"], 0.001)
	assert.InDelta(t, 1.0, rates["RUB"], 0.001)

	require.NoError(t, sitesetting_model.Set(ctx, map[string]string{"rate_USD": "92,5", "site_title": " Новое "}))
	all, err := sitesetting_model.All(ctx)
	require.NoError(t, err)
	assert.Equal(t, "92.5", all["rate_USD"])
	assert.Equal(t, "Новое", all["site_title"])

	require.ErrorIs(t, sitesetting_model.Set(ctx, map[string]string{"rate_EUR": "-1"}), db.ErrInvalidArgument)
}
