package deepseek

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultModelIDs(t *testing.T) {
	models := DefaultModelIDs()
	require.Equal(t, []string{"deepseek-chat", "deepseek-reasoner"}, models)

	models[0] = "changed"
	require.Equal(t, "deepseek-chat", DefaultModelIDs()[0])
}
