package image

import (
	"testing"

	"github.com/stretchr/testify/require"

	kargoapi "github.com/akuity/kargo/api/v1alpha1"
)

func TestNewLexicalSelector(t *testing.T) {
	testCases := []struct {
		name       string
		sub        kargoapi.ImageSubscription
		assertions func(*testing.T, Selector, error)
	}{
		{
			name: "error building tag based selector",
			sub:  kargoapi.ImageSubscription{}, // No RepoURL
			assertions: func(t *testing.T, _ Selector, err error) {
				require.ErrorContains(t, err, "error building tag based selector")
			},
		},
		{
			name: "success",
			sub: kargoapi.ImageSubscription{
				RepoURL:    "example/image",
				Constraint: "latest",
			},
			assertions: func(t *testing.T, s Selector, err error) {
				require.NoError(t, err)
				l, ok := s.(*lexicalSelector)
				require.True(t, ok)
				require.NotNil(t, l.tagBasedSelector)
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			s, err := newLexicalSelector(testCase.sub, nil)
			testCase.assertions(t, s, err)
		})
	}
}

func Test_lexicalSelector_sortTags(t *testing.T) {
	testCases := []struct {
		name     string
		unsorted []string
		expected []string
	}{
		{
			name: "calver with dots YYYY.M.D format",
			unsorted: []string{
				"2024.1.1",
				"2024.12.1",
				"2024.2.15",
				"2025.1.1",
				"2024.2.1",
			},
			expected: []string{
				"2025.1.1",
				"2024.12.1",
				"2024.2.15",
				"2024.2.1",
				"2024.1.1",
			},
		},
		{
			name: "calver with dots YYYY.MM.DD format",
			unsorted: []string{
				"2024.01.01",
				"2024.12.01",
				"2024.02.15",
				"2025.01.01",
				"2024.02.01",
			},
			expected: []string{
				"2025.01.01",
				"2024.12.01",
				"2024.02.15",
				"2024.02.01",
				"2024.01.01",
			},
		},
		{
			name: "calver with dashes YYYY-MM-DD format",
			unsorted: []string{
				"2024-01-01",
				"2024-12-01",
				"2024-02-15",
				"2025-01-01",
				"2024-02-01",
			},
			expected: []string{
				"2025-01-01",
				"2024-12-01",
				"2024-02-15",
				"2024-02-01",
				"2024-01-01",
			},
		},
		{
			name: "compact calver YYYYMMDD format",
			unsorted: []string{
				"20240101",
				"20241201",
				"20240215",
				"20250101",
				"20240201",
			},
			expected: []string{
				"20250101",
				"20241201",
				"20240215",
				"20240201",
				"20240101",
			},
		},
		{
			name: "mixed numeric tags",
			unsorted: []string{
				"v1.2.3",
				"v1.10.1",
				"v1.2.10",
				"v2.1.0",
				"v1.9.5",
			},
			expected: []string{
				"v2.1.0",
				"v1.10.1",
				"v1.9.5",
				"v1.2.10",
				"v1.2.3",
			},
		},
		{
			name: "calver with build metadata",
			unsorted: []string{
				"2024.1.1-build.123",
				"2024.12.1-build.456",
				"2024.2.1-build.789",
			},
			expected: []string{
				"2024.12.1-build.456",
				"2024.2.1-build.789",
				"2024.1.1-build.123",
			},
		},
		{
			name: "simple string tags (backward compatibility)",
			unsorted: []string{
				"latest",
				"stable",
				"dev",
				"beta",
			},
			expected: []string{
				"stable",
				"latest",
				"dev",
				"beta",
			},
		},
		{
			name: "numeric strings",
			unsorted: []string{
				"1",
				"10",
				"2",
				"20",
				"3",
			},
			expected: []string{
				"20",
				"10",
				"3",
				"2",
				"1",
			},
		},
		{
			name: "mixed format - numbers and strings",
			unsorted: []string{
				"v1.2",
				"v10.1",
				"latest",
				"v2.0",
			},
			expected: []string{
				"v10.1",
				"v2.0",
				"v1.2",
				"latest",
			},
		},
		{
			name:     "edge case: empty list",
			unsorted: []string{},
			expected: []string{},
		},
		{
			name:     "edge case: single element",
			unsorted: []string{"v1.0.0"},
			expected: []string{"v1.0.0"},
		},
		{
			name: "edge case: large numbers",
			unsorted: []string{
				"2024.1.1",
				"2024.1000.1",
				"2024.999.1",
				"2024.10.1",
			},
			expected: []string{
				"2024.1000.1",
				"2024.999.1",
				"2024.10.1",
				"2024.1.1",
			},
		},
		{
			name: "edge case: leading zeros",
			unsorted: []string{
				"2024.01.01",
				"2024.1.1",
				"2024.001.001",
			},
			expected: []string{
				"2024.001.001",
				"2024.01.01",
				"2024.1.1",
			},
		},
		{
			name: "real world calver examples",
			unsorted: []string{
				"2024.12.15",
				"2024.1.1",
				"2024.11.30",
				"2025.1.5",
				"2024.2.28",
			},
			expected: []string{
				"2025.1.5",
				"2024.12.15",
				"2024.11.30",
				"2024.2.28",
				"2024.1.1",
			},
		},
		{
			name: "docs example: nightly-yyyymmdd format",
			unsorted: []string{
				"nightly-20240101",
				"nightly-20241201",
				"nightly-20240215",
				"nightly-20250101",
				"nightly-20240201",
			},
			expected: []string{
				"nightly-20250101",
				"nightly-20241201",
				"nightly-20240215",
				"nightly-20240201",
				"nightly-20240101",
			},
		},
		{
			name: "edge case: numbers exceeding uint64",
			unsorted: []string{
				"v99999999999999999999.1.0",
				"v88888888888888888888.2.0",
				"v77777777777777777777.1.0",
			},
			expected: []string{
				"v99999999999999999999.1.0",
				"v88888888888888888888.2.0",
				"v77777777777777777777.1.0",
			},
		},
		{
			name: "edge case: mixed uint64 overflow and normal numbers",
			unsorted: []string{
				"v1.0.0",
				"v99999999999999999999.0.0",
				"v2.0.0",
			},
			expected: []string{
				"v99999999999999999999.0.0",
				"v2.0.0",
				"v1.0.0",
			},
		},
		{
			name: "edge case: pure numeric vs pure string comparison",
			unsorted: []string{
				"123",
				"abc",
				"456",
				"xyz",
			},
			expected: []string{
				"456",
				"123",
				"xyz",
				"abc",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			selector := &lexicalSelector{}
			sorted := selector.sortTags(testCase.unsorted)
			require.Equal(t, testCase.expected, sorted)
		})
	}
}
