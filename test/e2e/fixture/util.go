package fixture

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"golang.org/x/sync/errgroup"
)

var (
	matchFirstCap = regexp.MustCompile("(.)([A-Z][a-z]+)")
	matchAllCap   = regexp.MustCompile("([a-z0-9])([A-Z])")
)

// returns dns friends string which is no longer than 63 characters and has specified postfix at the end
func DnsFriendly(str string, postfix string) string { //nolint:revive //FIXME(var-naming)
	str = matchFirstCap.ReplaceAllString(str, "${1}-${2}")
	str = matchAllCap.ReplaceAllString(str, "${1}-${2}")
	str = strings.ToLower(str)

	if diff := len(str) + len(postfix) - 63; diff > 0 {
		str = str[:len(str)-diff]
	}
	return str + postfix
}

func RunFunctionsInParallelAndCheckErrors(t *testing.T, functions []func() error) {
	t.Helper()
	var eg errgroup.Group
	for _, function := range functions {
		eg.Go(function)
	}
	require.NoError(t, eg.Wait())
}
