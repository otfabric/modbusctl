package config

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestRegisterFlags_uintAndStringSliceDefaults(t *testing.T) {
	t.Parallel()
	type cfg struct {
		N   uint     `flag:"n" desc:"count" env:"MODBUSCTL_TEST_N"`
		Sub []string `flag:"sub" desc:"subs" env:"MODBUSCTL_TEST_SUB"`
	}
	c := cfg{
		N:   3,
		Sub: []string{"a", "b"},
	}
	cmd := cobra.Command{}
	RegisterFlags(&cmd, &c)
	require.NoError(t, cmd.Flags().Parse([]string{"--n", "9"}))
	require.Equal(t, uint(9), c.N)
	require.Equal(t, []string{"a", "b"}, c.Sub)
	require.NoError(t, cmd.Flags().Parse([]string{"--sub", "x", "--sub", "y"}))
	require.Equal(t, []string{"x", "y"}, c.Sub)
}

func TestLoadFromEnv_uint(t *testing.T) {
	type cfg struct {
		N uint `env:"MODBUSCTL_TEST_LOAD_UINT"`
	}
	c := cfg{}
	t.Setenv("MODBUSCTL_TEST_LOAD_UINT", "42")
	require.NoError(t, LoadFromEnv(&c))
	require.Equal(t, uint(42), c.N)
}

func TestLoadFromEnv_invalidUint(t *testing.T) {
	type cfg struct {
		N uint `env:"MODBUSCTL_TEST_BAD_UINT"`
	}
	c := cfg{}
	t.Setenv("MODBUSCTL_TEST_BAD_UINT", "not-a-number")
	require.Error(t, LoadFromEnv(&c))
}
