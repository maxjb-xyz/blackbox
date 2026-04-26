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

func TestKnownSourceTypes_SingletonsExistInModelAllowlists(t *testing.T) {
	agentSingletons := map[string]struct{}{}
	for _, sourceType := range models.GetAgentScopedSingletonSourceTypes() {
		agentSingletons[sourceType] = struct{}{}
	}

	serverSingletons := map[string]struct{}{}
	for _, sourceType := range models.GetServerScopedSingletonSourceTypes() {
		serverSingletons[sourceType] = struct{}{}
	}

	for _, typeDef := range knownSourceTypes {
		if !typeDef.Singleton {
			continue
		}

		switch typeDef.Scope {
		case models.ScopeAgent:
			_, ok := agentSingletons[typeDef.Type]
			require.Truef(t, ok, "missing agent singleton allowlist entry for %s", typeDef.Type)
		case models.ScopeServer:
			_, ok := serverSingletons[typeDef.Type]
			require.Truef(t, ok, "missing server singleton allowlist entry for %s", typeDef.Type)
		}
	}
}
