package memory

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func makeEpisodes(ids ...string) []Episode {
	var eps []Episode
	for _, id := range ids {
		eps = append(eps, Episode{ID: id, Content: "content-" + id})
	}
	return eps
}

func TestRRFMerge(t *testing.T) {
	store := &PgEpisodicStore{
		ftsWeight:    0.3,
		vectorWeight: 0.7,
		rrfK:         60,
	}

	fts := makeEpisodes("a", "b", "c") // a ranked 1st, b 2nd, c 3rd
	vec := makeEpisodes("b", "d", "a") // b ranked 1st, d 2nd, a 3rd

	result := store.rrfMerge(fts, vec, 4)

	ids := make([]string, len(result))
	for i, ep := range result {
		ids[i] = ep.ID
	}

	// a: 0.3/61 + 0.7/63 = 0.004918 + 0.011111 = 0.016029
	// b: 0.3/62 + 0.7/61 = 0.004839 + 0.011475 = 0.016314
	// c: 0.3/63 = 0.004762
	// d: 0.7/62 = 0.011290
	// Expected order: b, a, d, c
	assert.Equal(t, "b", ids[0])
	assert.Equal(t, "a", ids[1])
	assert.Equal(t, "d", ids[2])
	assert.Equal(t, "c", ids[3])
}

func TestRRFMerge_EmptyInputs(t *testing.T) {
	store := &PgEpisodicStore{ftsWeight: 0.3, vectorWeight: 0.7, rrfK: 60}

	// Both empty
	result := store.rrfMerge(nil, nil, 5)
	assert.Empty(t, result)

	// Only FTS
	fts := makeEpisodes("x", "y")
	result = store.rrfMerge(fts, nil, 5)
	assert.Len(t, result, 2)

	// Only Vector
	result = store.rrfMerge(nil, makeEpisodes("z"), 5)
	assert.Len(t, result, 1)
}

func TestRRFMerge_LimitRespected(t *testing.T) {
	store := &PgEpisodicStore{ftsWeight: 0.3, vectorWeight: 0.7, rrfK: 60}

	fts := makeEpisodes("a", "b", "c", "d", "e")
	vec := makeEpisodes("f", "g", "h")

	result := store.rrfMerge(fts, vec, 3)
	assert.Len(t, result, 3)
}
