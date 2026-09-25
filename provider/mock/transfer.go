package mock

import (
	"iter"

	"go.rtnl.ai/endeavor/pkg/horizon"
)

const (
	Close  = "Close"
	Agent  = "Agent"
	Agents = "Agents"
	Task   = "Task"
	Tasks  = "Tasks"
	Err    = "Err"
)

// Mock exporter for testing the horizon package.
type Exporter struct {
	Mock
	OnClose  func() error
	OnAgents func() iter.Seq[*horizon.Agent]
	OnTasks  func() iter.Seq[*horizon.Task]
	OnErr    func() error
}

var _ horizon.Exporter = (*Exporter)(nil)

func (e *Exporter) Close() error {
	e.call(Close)
	if e.OnClose == nil {
		return nil
	}
	return e.OnClose()
}

func (e *Exporter) Agents() iter.Seq[*horizon.Agent] {
	e.call(Agents)
	if e.OnAgents == nil {
		return empty[*horizon.Agent]()
	}
	return e.OnAgents()
}

func (e *Exporter) Tasks() iter.Seq[*horizon.Task] {
	e.call(Tasks)
	if e.OnTasks == nil {
		return empty[*horizon.Task]()
	}
	return e.OnTasks()
}

func (e *Exporter) Err() error {
	e.call(Err)
	if e.OnErr == nil {
		return nil
	}
	return e.OnErr()
}

// Mock importer for testing the horizon package.
type Importer struct {
	Mock
	OnClose func() error
	OnAgent func(...*horizon.Agent) error
	OnTask  func(...*horizon.Task) error
}

var _ horizon.Importer = (*Importer)(nil)

func (i *Importer) Close() error {
	i.call(Close)
	if i.OnClose == nil {
		return nil
	}
	return i.OnClose()
}

func (i *Importer) Agent(agents ...*horizon.Agent) error {
	i.call(Agent)
	if i.OnAgent == nil {
		return nil
	}
	return i.OnAgent(agents...)
}

func (i *Importer) Task(tasks ...*horizon.Task) error {
	i.call(Task)
	if i.OnTask == nil {
		return nil
	}
	return i.OnTask(tasks...)
}

// Return an empty sequence of the given type.
func empty[T any]() iter.Seq[T] {
	return func(yield func(T) bool) {}
}
