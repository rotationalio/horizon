package horizon

import (
	"errors"
	"sort"
	"strings"
)

// Router is a space efficient radix tree used for assigning path components to tasks.
type Router struct {
	root *node
	size int
}

// Returns the number of elements in the router.
func (r *Router) Size() int {
	return r.size
}

// Insert or replace a task at the given path. Returns true if the task was replaced.
// NOTE: the path should be an absolute path with a leading `/` character. The path
// should also be URL safe (e.g. url encoded) with no query string.
func (r *Router) Insert(path string, task *Task) bool {
	// Prepare the router for the insert operation.
	if r.root == nil {
		r.root = &node{
			prefix: "/",
			leaf:   nil,
		}
	}

	// Prepare the path for the insert operation.
	if len(path) > 0 && path[0] == '/' {
		path = path[1:]
	}

	var parent *node
	current := r.root
	search := path

	for {
		// Handle key exhaustion
		if len(search) == 0 {
			if current.isLeaf() {
				current.leaf.task = task
				return true
			}
			current.leaf = &leaf{
				path: path,
				task: task,
			}
			r.size++
			return false
		}

		// Look for the edge with the first character of the search key.
		parent = current
		current = current.get(search[0])

		// If there is no edge, create one
		if current == nil {
			e := edge{
				label: search[0],
				node: &node{
					prefix: search,
					leaf: &leaf{
						path: path,
						task: task,
					},
				},
			}

			parent.append(e)
			r.size++
			return false
		}

		// Determine the longest prefix of the search key on match
		common := longestCommonPrefix(search, current.prefix)
		if common == len(current.prefix) {
			search = search[common:]
			continue
		}

		// Split the node
		r.size++
		child := &node{
			prefix: search[:common],
		}
		parent.update(search[0], child)

		// Restore the existing node
		child.append(edge{
			label: current.prefix[common],
			node:  current,
		})
		current.prefix = current.prefix[common:]

		// Create a new leaf node
		leaf := &leaf{
			path: path,
			task: task,
		}

		// If the new key is a subset add it to the current node
		search = search[common:]
		if len(search) == 0 {
			child.leaf = leaf
			return false
		}

		// Otherwise create a new edge for the node
		child.append(edge{
			label: search[0],
			node: &node{
				prefix: search,
				leaf:   leaf,
			},
		})
		return false
	}
}

// Get the task at the specified path. Returns the task and true if the task was found.
func (r *Router) Get(path string) (*Task, bool) {
	// Check if the router is empty.
	if r.root == nil {
		return nil, false
	}

	current := r.root
	search := path

	// Prepare the path for the get operation.
	if len(search) > 0 && search[0] == '/' {
		search = search[1:]
	}

	for {
		// Handle key exhaustion
		if len(search) == 0 {
			if current.isLeaf() {
				return current.leaf.task, true
			}
			break
		}

		// Look for an edge
		current = current.get(search[0])
		if current == nil {
			break
		}

		if strings.HasPrefix(search, current.prefix) {
			search = search[len(current.prefix):]
		} else {
			break
		}
	}
	return nil, false
}

// Remove the task at the specified path. Returns true if a task was removed.
func (r *Router) Remove(path string) bool {
	// Check if the router is empty.
	if r.root == nil {
		return false
	}

	var (
		parent *node
		label  byte
	)

	current := r.root
	search := path

	// Prepare the path for the remove operation.
	if len(search) > 0 && search[0] == '/' {
		search = search[1:]
	}

	for {
		// Handle key exhaustion
		if len(search) == 0 {
			if !current.isLeaf() {
				break
			}
			goto DELETE
		}

		// Look for an edge
		parent = current
		label = search[0]
		current = current.get(label)

		if current == nil {
			break
		}

		// Consume the search prefix
		if strings.HasPrefix(search, current.prefix) {
			search = search[len(current.prefix):]
		} else {
			break
		}
	}
	return false

DELETE:
	current.leaf = nil
	r.size--

	// Check if we should delete the current node from the parent
	if parent != nil && len(current.edges) == 0 {
		parent.remove(label)
	}

	// Check if we should merge the current node
	if current != r.root && len(current.edges) == 1 {
		current.merge()
	}

	// Check if we should merge the parent's other child
	if parent != nil && parent != r.root && len(parent.edges) == 1 && !parent.isLeaf() {
		parent.merge()
	}

	return true
}

//============================================================================
// Implementation of Radix Tree for path to Task lookups.
//============================================================================

type node struct {
	prefix string
	edges  edges
	leaf   *leaf
}

type edge struct {
	label byte
	node  *node
}

type leaf struct {
	path string
	task *Task
}

func (n *node) isLeaf() bool {
	return n.leaf != nil
}

func (n *node) append(e edge) {
	l := len(n.edges)
	i := sort.Search(l, func(i int) bool {
		return n.edges[i].label >= e.label
	})

	n.edges = append(n.edges, edge{})
	copy(n.edges[i+1:], n.edges[i:])
	n.edges[i] = e
}

func (n *node) update(label byte, node *node) {
	l := len(n.edges)
	i := sort.Search(l, func(i int) bool {
		return n.edges[i].label >= label
	})

	if i < l && n.edges[i].label == label {
		n.edges[i].node = node
		return
	}
	panic(errors.New("cannot update missing edge"))
}

func (n *node) get(label byte) *node {
	l := len(n.edges)
	i := sort.Search(l, func(i int) bool {
		return n.edges[i].label >= label
	})

	if i < l && n.edges[i].label == label {
		return n.edges[i].node
	}
	return nil
}

func (n *node) remove(label byte) {
	l := len(n.edges)
	i := sort.Search(l, func(i int) bool {
		return n.edges[i].label >= label
	})

	if i < l && n.edges[i].label == label {
		copy(n.edges[i:], n.edges[i+1:])
		n.edges[len(n.edges)-1] = edge{}
		n.edges = n.edges[:len(n.edges)-1]
	}
}

func (n *node) merge() {
	e := n.edges[0]
	child := e.node
	n.prefix = n.prefix + child.prefix
	n.leaf = child.leaf
	n.edges = child.edges
}

type edges []edge

func (e edges) Less(i, j int) bool {
	return e[i].label < e[j].label
}

func (e edges) Swap(i, j int) {
	e[i], e[j] = e[j], e[i]
}

func (e edges) Len() int {
	return len(e)
}

func (e edges) Sort() {
	sort.Sort(e)
}

//============================================================================
// Helper functions
//============================================================================

func longestCommonPrefix(a, b string) int {
	i := 0
	max := min(len(a), len(b))
	for i < max && a[i] == b[i] {
		i++
	}
	return i
}
