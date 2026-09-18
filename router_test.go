package horizon_test

import (
	"iter"
	"math/rand"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon"
	"go.rtnl.ai/x/semver"
	"go.rtnl.ai/x/slugify"
)

// Names for the various components of the router.
var (
	environments = []string{
		"development",
		"demo",
		"staging",
		"production",
	}
	agents = []string{
		"compass",
		"aurora",
		"nova",
		"odyssey",
		"polaris",
		"sagittarius",
		"capella",
		"sirius",
		"betelgeuse",
		"antares",
		"rigel",
		"procyon",
		"aldebaran",
		"pollux",
		"castor",
		"athena",
		"ares",
		"yankee",
		"franklin",
		"ganymede",
		"hercules",
		"indigo",
		"jupiter",
		"kali",
		"luna",
		"mercury",
		"neptune",
		"oberon",
		"palladium",
		"quasar",
		"rook",
		"titan",
		"ursa",
		"valkyrie",
		"wolfram",
		"zephyr",
		"xenia",
		"xanthos",
		"yggdrasil",
		"excalibur",
		"falcon",
		"zulu",
		"zapata",
	}
	tasks = [][]string{
		{
			"Apply", "Summarize", "Analyze", "Check", "Review", "Inspect", "Verify",
			"Approve", "Complete", "Submit", "Monitor", "Execute", "Report", "Confirm",
			"Archive", "Uninstall", "Publish", "Deploy", "Restart", "Rollback",
			"Update", "Patch", "Upgrade", "Downgrade", "Generate", "Delete", "Unpublish",
			"Subscribe", "Refresh", "Extract", "Transform", "Load", "Import", "Export",
			"Copy", "Move", "Rename", "Purge", "Restore", "Recover", "Repair", "List",
			"Optimize", "Classify", "Identify", "Recognize", "Detect", "Locate", "Track",
			"Categorize", "Tag", "Label", "Define", "Collate", "Filter", "Enrich",
			"Enhance", "Improve", "Optimize", "Refine", "Clean", "Organize", "Structure",
			"Format", "Sanitize", "Validate", "Normalize", "Standardize", "Consolidate",
			"Aggregate", "Combine", "Merge", "Split", "Divide", "Segment", "Slice",
			"Dice", "Chunk", "Piece", "Part", "Section", "Transcribe", "Translate",
		},
		{
			"Entities", "Documents", "News Articles", "Posts", "Comments", "Emails",
			"Tweet", "Messages", "Alerts", "Notifications", "Logs", "Events", "Errors",
			"Statistics", "Incidents", "Voicemails", "Images", "Volumes", "Books", "Texts",
			"Recordings", "Telemetry", "Videos", "Metrics", "Statistics", "Videos",
			"Pictures", "Data", "Files", "Folders", "Directories", "Databases", "Tables",
			"Modules", "Devices", "Systems", "Processes", "Threads", "Services",
			"Applications", "Orders", "Products", "Items", "Customers", "Users", "Profiles",
			"Accounts", "Roles", "Permissions", "Policies", "Settings", "Preferences",
			"Configurations", "Parameters", "Options", "Values", "Invoices", "Payments",
			"Transfers", "Deposits", "Withdrawals", "Balances", "Statements", "Reports",
			"Dashboards", "Charts", "Graphs", "Maps", "Tables", "Lists", "Grids", "Cards",
			"Forms", "Fields", "Labels", "Titles", "Descriptions", "Summaries", "Synopses",
			"Abstracts", "Conclusions", "Recommendations", "Suggestions", "Observations",
			"Findings", "Insights", "Opinions", "Beliefs", "Attitudes", "Preferences",
		},
	}
)

func TestRouter(t *testing.T) {
	t.Run("Simple", func(t *testing.T) {
		taska := &horizon.Task{Name: "Task A"}
		taskb := &horizon.Task{Name: "Task B"}
		taskc := &horizon.Task{Name: "Task C"}

		r := &horizon.Router{}
		require.False(t, r.Insert("/", taska))
		require.False(t, r.Insert("/tasks/a", taska))
		require.False(t, r.Insert("/tasks/b", taskb))
		require.False(t, r.Insert("/tasks/c", taskc))

		cmpt, ok := r.Get("/")
		require.True(t, ok)
		require.Equal(t, taska, cmpt)

		cmpt, ok = r.Get("/tasks/a")
		require.True(t, ok)
		require.Equal(t, taska, cmpt)

		cmpt, ok = r.Get("/tasks/b")
		require.True(t, ok)
		require.Equal(t, taskb, cmpt)

		cmpt, ok = r.Get("/tasks/c")
		require.True(t, ok)
		require.Equal(t, taskc, cmpt)

		require.Equal(t, 3, r.Size())
	})

	t.Run("Complex", func(t *testing.T) {
		r := &horizon.Router{}
		paths := make([]*taskPath, 0, 1024)
		for path := range randomPaths(1024) {
			paths = append(paths, path)
			r.Insert(path.path, path.task)
		}

		for _, path := range paths {
			cmpt, ok := r.Get(path.path)
			require.True(t, ok)
			require.Equal(t, path.task, cmpt)
		}

		require.Equal(t, len(paths), r.Size())

		for _, path := range paths {
			require.True(t, r.Remove(path.path))
		}

		require.Equal(t, 0, r.Size())
	})

	t.Run("Update", func(t *testing.T) {
		r := &horizon.Router{}
		task := &horizon.Task{Name: randomTask()}

		require.False(t, r.Insert("/path/to/task", task))
		cmpt, ok := r.Get("/path/to/task")
		require.True(t, ok)
		require.Equal(t, task, cmpt)

		replaced := &horizon.Task{Name: randomTask()}
		require.True(t, r.Insert("/path/to/task", replaced))

		cmpt, ok = r.Get("/path/to/task")
		require.True(t, ok)
		require.Equal(t, replaced, cmpt)
	})

	t.Run("Remove", func(t *testing.T) {
		r := &horizon.Router{}
		task := &horizon.Task{Name: randomTask()}
		require.False(t, r.Insert("/path/to/task", task))
		require.True(t, r.Remove("/path/to/task"))
		_, ok := r.Get("/path/to/task")
		require.False(t, ok)
	})

	t.Run("Root", func(t *testing.T) {
		// There should be no panic if the router is empty
		r := &horizon.Router{}
		require.False(t, r.Remove(""))

		// Router should still be empty and there should be no panic.
		_, ok := r.Get("")
		require.False(t, ok)

		// Should be able to insert into an empty router.
		task := &horizon.Task{Name: "test"}
		require.False(t, r.Insert("/", task))
	})

	t.Run("RootPath", func(t *testing.T) {
		testf := func(base string, root string) func(t *testing.T) {
			return func(t *testing.T) {
				r := &horizon.Router{}
				task := &horizon.Task{Name: randomTask()}
				require.False(t, r.Insert(base, task))

				cmpt, ok := r.Get(base)
				require.True(t, ok)
				require.Equal(t, task, cmpt)

				cmpt, ok = r.Get(root)
				require.True(t, ok)
				require.Equal(t, task, cmpt)

				require.True(t, r.Remove(base))
				_, ok = r.Get(base)
				require.False(t, ok)

				_, ok = r.Get(root)
				require.False(t, ok)
			}
		}

		t.Run("Slash", testf("/", ""))

		t.Run("Empty", testf("", "/"))
	})
}

//============================================================================
// Create Random, Realistic Horizon Task Paths
//============================================================================

type taskPath struct {
	task *horizon.Task
	path string
}

func randomPaths(n int) iter.Seq[*taskPath] {
	makep := func(task *horizon.Task, pathParts ...string) *taskPath {
		return &taskPath{
			task: task,
			path: "/" + strings.Join(pathParts, "/"),
		}
	}

	return func(yield func(*taskPath) bool) {
		count := 0
		for {
			if count >= n {
				return
			}

			agent := randomAgent()
			envs := randomEnvironments()
			ntasks := rand.Intn(10) + 1

			for range ntasks {
				version := &semver.Version{Major: 1}
				task := &horizon.Task{Name: randomTask()}
				slug := slugify.Slugify(task.Name)

				// Yield the latest task version
				if !yield(makep(task, agent, slug)) {
					return
				}

				count++
				if count >= n {
					return
				}

				// Yield the latest task version for each environment
				for _, env := range envs {
					if !yield(makep(task, env, agent, slug)) {
						return
					}

					count++
					if count >= n {
						return
					}
				}

				// Yield multiple versions of the task for each environment
				nversions := rand.Intn(10) + 1
				for range nversions {
					if !yield(makep(task, agent, slug, version.String())) {
						return
					}

					count++
					if count >= n {
						return
					}

					for _, env := range envs {
						if !yield(makep(task, env, agent, slug, version.String())) {
							return
						}

						count++
						if count >= n {
							return
						}
					}

					randomizeVersion(version)
				}
			}
		}
	}
}

func randomEnvironments() []string {
	envs := make([]string, 0, len(environments))
	for _, env := range environments {
		if rand.Float64() < 0.25 {
			envs = append(envs, env)
		}
	}
	return envs
}

func randomAgent() string {
	return agents[rand.Intn(len(agents))]
}

func randomTask() string {
	return tasks[0][rand.Intn(len(tasks[0]))] + " " + tasks[1][rand.Intn(len(tasks[1]))]
}

func randomizeVersion(version *semver.Version) {
	cf := rand.Float64()
	switch {
	case cf < 0.05:
		version.Major++
	case cf < .65:
		version.Minor++
	default:
		version.Patch++
	}
}
