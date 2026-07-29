package idgen_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/NoxiouSi/eino-risk-qa/internal/infra/idgen"
)

func TestUUIDGenerator_NewBatchID_HasPrefixAndIsUnique(t *testing.T) {
	g := idgen.NewUUIDGenerator()

	a := g.NewBatchID()
	b := g.NewBatchID()

	assert.True(t, strings.HasPrefix(a, "batch_"))
	assert.NotEqual(t, a, b)
}

func TestUUIDGenerator_NewSessionID_HasPrefixAndIsUnique(t *testing.T) {
	g := idgen.NewUUIDGenerator()

	a := g.NewSessionID()
	b := g.NewSessionID()

	assert.True(t, strings.HasPrefix(a, "sess_"))
	assert.NotEqual(t, a, b)
}
