package shortener

import (
	"crypto/rand"
	"math/big"
	"strings"
)

var aliasWordGroups = [][]string{
	{"amber", "brisk", "calm", "daring", "ember", "fancy", "gentle", "honest", "ivory", "jolly", "kind", "lively", "mellow", "nimble", "open", "proud"},
	{"apple", "beacon", "cedar", "dawn", "echo", "field", "grove", "harbor", "island", "jungle", "lagoon", "meadow", "north", "oasis", "prairie", "river"},
	{"breeze", "comet", "feather", "glimmer", "lantern", "meadow", "pebble", "rocket", "signal", "sprout", "summit", "sunbeam", "thunder", "trail", "willow", "zephyr"},
	{"anchor", "bridge", "cabin", "forest", "garden", "market", "orange", "planet", "shadow", "silver", "sunset", "temple", "valley", "violet", "window", "winter"},
}

func generateAlias() (string, error) {
	parts := make([]string, 0, len(aliasWordGroups))
	for _, group := range aliasWordGroups {
		word, err := randomWord(group)
		if err != nil {
			return "", err
		}
		parts = append(parts, word)
	}
	return strings.Join(parts, "-"), nil
}

func randomWord(words []string) (string, error) {
	index, err := rand.Int(rand.Reader, big.NewInt(int64(len(words))))
	if err != nil {
		return "", err
	}
	return words[index.Int64()], nil
}
