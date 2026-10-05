package archive_test

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/archive"
	"go.rtnl.ai/horizon/task"
	"go.rtnl.ai/ulid"
	"go.rtnl.ai/x/semver"
)

func TestTransfer(t *testing.T) {
	PrepareFixtures(t)

	// This is an end to end test of the Transfer function that also exercises most of
	// the zip methods and functions for the importer and exporter (e.g. in the happy path).
	src, err := archive.Open("testdata/travel.zip", archive.ModeRead)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, src.Close()) })

	dst, err := archive.Open(filepath.Join(t.TempDir(), "horizon_transfer_test.zip"), archive.ModeWrite)
	require.NoError(t, err)

	require.NoError(t, task.Transfer(src, dst))
	require.NoError(t, dst.Close())
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{input: "Hello-World", expected: "hello_world"},
		{input: "Hello_World", expected: "hello_world"},
		{input: "_hello-World-", expected: "hello_world"},
		{input: "/hello-world", expected: "hello_world"},
		{input: "---hello-world-------", expected: "hello_world"},
		{input: "\\/-_hello-world\\/-", expected: "hello_world"},
	}

	for i, tc := range tests {
		t.Run(fmt.Sprintf("TC%02d", i), func(t *testing.T) {
			require.Equal(t, tc.expected, archive.Normalize(tc.input))
		})
	}
}

//============================================================================
// Fixtures
//============================================================================

func PrepareFixtures(t *testing.T) {
	t.Helper()
	if !FileExists("testdata/travel.zip") {
		require.NoError(t, CreateCompass())
	}
}

func FileExists(path string) bool {
	_, err := os.Stat(path)
	return !errors.Is(err, fs.ErrNotExist)
}

func CreateCompass() (err error) {
	var out *archive.File
	if out, err = archive.Open("testdata/travel.zip", archive.ModeWrite); err != nil {
		return err
	}

	if err = out.Task(travelTasks...); err != nil {
		return err
	}

	return out.Close()
}

var travelTasks = []*task.Task{
	{
		ID:          ulid.MustParse("01KN8FXANHMK4HAKKH0T4BKJP6"),
		Slug:        "relevant-flights",
		Name:        "Relevant Flights",
		Description: "Find a series of flights that meet the user's preferences and rank according to price.",
		Version:     semver.MustParse("1.0.0"),
	},
	{
		ID:          ulid.MustParse("01KN8GADJYPMTQN5VNFC3NEW2E"),
		Slug:        "rank-by-comfort",
		Name:        "Rank by Comfort",
		Description: "Given a list of flights, rank them according to a comfort metric described by airport, flight, and aircraft information.",
		Version:     semver.MustParse("1.1.0"),
	},
	{
		ID:          ulid.MustParse("01KN8G5XDYDY09PBB3NWB331VK"),
		Slug:        "rank-by-comfort-simple",
		Name:        "Rank by Comfort",
		Description: "Given a list of flights, rank them according to a comfort metric based on flight duration and number of layovers.",
		Version:     semver.MustParse("1.0.0"),
	},
	{
		ID:          ulid.MustParse("01KN8G6QN7TR3VVT7CFNZ88H77"),
		Slug:        "propose-itineraries",
		Name:        "Propose Itineraries",
		Description: "Given a list of flights ranked by comfort and price, propose 2-3 options of itineraries for the user to evaluate.",
		Version:     semver.MustParse("1.0.0"),
	},
}
