package handlers

import (
	"fmt"
	"testing"

	"blackbox/server/internal/models"
	"github.com/stretchr/testify/require"
)

func TestKnownSourceTypes_SingletonFlagsMatchModelRegistry(t *testing.T) {
	for _, typeDef := range knownSourceTypes {
		typeDef := typeDef
		t.Run(fmt.Sprintf("%s/%s", typeDef.Scope, typeDef.Type), func(t *testing.T) {
			require.Equalf(
				t,
				models.IsSingletonSourceType(typeDef.Scope, typeDef.Type),
				typeDef.Singleton,
				"singleton mismatch for %s/%s",
				typeDef.Scope,
				typeDef.Type,
			)
		})
	}
}
