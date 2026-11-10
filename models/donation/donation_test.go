package donation_test

import (
	"testing"

	"git.cu3rd.ru/cu3rd/dash/models/db"
	donation_model "git.cu3rd.ru/cu3rd/dash/models/donation"
	"git.cu3rd.ru/cu3rd/dash/models/unittest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateIsIdempotentByExternalID(t *testing.T) {
	unittest.PrepareTestDatabase(t)
	ctx := t.Context()

	d := &donation_model.Donation{Donor: "kot", Amount: 1000, Currency: "USD", DonatedOn: "2026-07-01", Source: "boosty", ExternalID: "tx-1"}
	created, err := donation_model.Create(ctx, d)
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, int64(4), d.ID, "the existing row is returned")

	d = &donation_model.Donation{Amount: 500, Currency: "RUB", DonatedOn: "2026-11-01", Source: "boosty", ExternalID: "tx-2"}
	created, err = donation_model.Create(ctx, d)
	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, "Аноним", d.DisplayName())

	// manual donations have no external id and are never merged
	for range 2 {
		created, err = donation_model.Create(ctx, &donation_model.Donation{Amount: 100, Currency: "RUB", DonatedOn: "2026-11-01"})
		require.NoError(t, err)
		assert.True(t, created)
	}
	unittest.AssertCount(t, new(donation_model.Donation), 7)
}

func TestValidate(t *testing.T) {
	for _, d := range []*donation_model.Donation{
		{Amount: 0, Currency: "RUB", DonatedOn: "2026-11-01"},
		{Amount: 1, Currency: "XXX", DonatedOn: "2026-11-01"},
		{Amount: 1, Currency: "RUB", DonatedOn: "вчера"},
	} {
		_, err := donation_model.Create(t.Context(), d)
		require.ErrorIs(t, err, db.ErrInvalidArgument)
	}
}

func TestListPublicOnly(t *testing.T) {
	unittest.PrepareTestDatabase(t)

	all, err := donation_model.List(t.Context(), false)
	require.NoError(t, err)
	assert.Len(t, all, 4)
	public, err := donation_model.List(t.Context(), true)
	require.NoError(t, err)
	assert.Len(t, public, 3)
	assert.Equal(t, "Вася", public[0].Donor, "newest first")
}
