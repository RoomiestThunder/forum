package api

import (
	"os"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestMain(m *testing.M) {
	// Hash cost is irrelevant to what these tests check and dominates their
	// run time; see bcryptCost.
	bcryptCost = bcrypt.MinCost
	os.Exit(m.Run())
}
