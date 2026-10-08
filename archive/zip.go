package archive

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"iter"
	"os"
	"path/filepath"
	"strings"

	"go.rtnl.ai/horizon/task"
	"go.rtnl.ai/horizon/version"
	"go.rtnl.ai/ulid"
	"go.rtnl.ai/x/semver"
	yaml "gopkg.in/yaml.v2"
)

//============================================================================
// Zip File Import/Export Format for Tasks
//============================================================================

type Mode uint8

const (
	ModeRead   Mode = 'r'
	ModeWrite  Mode = 'w'
	ModeAppend Mode = 'a'

	// The default output type for exported tasks.
	DefaultExtension = ".json"
)

// File allows for the import and export of tasks from a zip file and
// implements both the Importer and Exporter interfaces. The mode the File is opened
// determines whether or not it is an importer or an exporter.
type File struct {
	mode     Mode
	path     string
	reader   *zip.ReadCloser
	writer   *zip.Writer
	manifest *Manifest
	err      error
	file     *os.File
}

var _ task.Importer = (*File)(nil)
var _ task.Exporter = (*File)(nil)

// Manifest is a record of the tasks available in the zip file.
type Manifest struct {
	Version semver.Version `json:"version" yaml:"version" msg:"version"` // the version of the manifest
	Tasks   []*Entry       `json:"tasks" yaml:"tasks" msg:"tasks"`       // the tasks available in the manifest
}

// Entry is a record of the path to the task in the zip file.
type Entry struct {
	ID   ulid.ULID `json:"id,omitempty" yaml:"id,omitempty" msg:"id,omitempty"`       // the ID of the entry
	Slug string    `json:"slug,omitempty" yaml:"slug,omitempty" msg:"slug,omitempty"` // the slug of the entry
	Path string    `json:"path" yaml:"path" msg:"path"`                               // the path of the entry
}

// Open a horizon zip file for reading, writing, or appending. If opened for reading,
// the file implements the Importer interface without error. If opened for writing or
// appending, the file implements the Exporter interface without error.
func Open(path string, mode Mode) (f *File, err error) {
	f = &File{
		mode: mode,
		path: path,
		err:  nil,
	}

	switch mode {
	case ModeRead:
		// Open the zip file for reading.
		if f.reader, err = zip.OpenReader(path); err != nil {
			return nil, err
		}
	case ModeWrite:
		// Create a new zip file for writing (any old file will be overwritten).
		if f.file, err = os.Create(path); err != nil {
			return nil, err
		}

		f.writer = zip.NewWriter(f.file)
		f.manifest = &Manifest{Version: version.Version(), Tasks: make([]*Entry, 0)}
	case ModeAppend:
		// Copy the existing zip file to a temporary file. Open the existing zip file
		// for writing (truncating it) then copy the contents to the new file, which is
		// now ready to have new agents and tasks added to it.
		//
		// If the above process fails at any point, make a good faith effort to restore
		// the original file.
		var (
			tmp     string
			restore func() error
		)

		if tmp, restore, err = tempArchive(path); err != nil {
			return nil, err
		}
		defer os.Remove(tmp)

		if f.file, err = os.Create(path); err != nil {
			return nil, errors.Join(err, restore())
		}

		f.writer = zip.NewWriter(f.file)
		f.manifest = &Manifest{Version: version.Version(), Tasks: make([]*Entry, 0)}

		if err = copyArchive(f.writer, tmp); err != nil {
			return nil, errors.Join(err, restore())
		}
	default:
		return nil, fmt.Errorf("unsupported mode: %q", mode)
	}
	return f, nil
}

// Close the file and clean up any resources.
// NOTE: in export mode the file must be closed before the archive is written to disk.
func (f *File) Close() (err error) {
	if f.reader != nil {
		if cerr := f.reader.Close(); cerr != nil {
			err = errors.Join(err, fmt.Errorf("failed to close reader: %w", cerr))
		}
		f.reader = nil
	}
	if f.writer != nil {
		// Save the manifest to the zip file.
		if werr := f.writeFile("manifest"+DefaultExtension, f.manifest); werr != nil {
			err = errors.Join(err, fmt.Errorf("failed to save manifest: %w", werr))
		}

		if cerr := f.writer.Close(); cerr != nil {
			err = errors.Join(err, fmt.Errorf("failed to close writer: %w", cerr))
		}
		f.writer = nil
	}
	if f.file != nil {
		if cerr := f.file.Close(); cerr != nil {
			err = errors.Join(err, fmt.Errorf("failed to close archive file: %w", cerr))
		}
		f.file = nil
	}
	return err
}

//============================================================================
// Importer Methods
//============================================================================

// Import tasks from the zip file as long as the file is open for reading.
// NOTE: this method is not safe for concurrent use.
func (f *File) Tasks() iter.Seq[*task.Task] {
	if f.mode != ModeRead {
		f.err = errors.Join(f.err, fmt.Errorf("cannot import tasks from a file in %q mode", f.mode))
		return empty[*task.Task]()
	}

	if f.reader == nil {
		f.err = errors.Join(f.err, ErrClosed)
		return empty[*task.Task]()
	}

	if f.manifest == nil {
		if err := f.loadManifest(); err != nil {
			f.err = errors.Join(f.err, err)
			return empty[*task.Task]()
		}
	}

	return func(yield func(*task.Task) bool) {
		for _, entry := range f.manifest.Tasks {
			task := &task.Task{}
			if err := f.readFile(entry.Path, task); err != nil {
				f.err = errors.Join(f.err, err)
				return
			}

			if task.ID.IsZero() && !entry.ID.IsZero() {
				task.ID = entry.ID
			} else if !task.ID.IsZero() && !entry.ID.IsZero() {
				if task.ID != entry.ID {
					f.err = errors.Join(f.err, fmt.Errorf("manifest task ID mismatch for task at %q", entry.Path))
					continue
				}
			}

			if task.Slug == "" && entry.Slug != "" {
				task.Slug = entry.Slug
			} else if task.Slug != "" && entry.Slug != "" {
				if task.Slug != entry.Slug {
					f.err = errors.Join(f.err, fmt.Errorf("manifest task slug mismatch for task at %q", entry.Path))
					continue
				}
			}

			if !yield(task) {
				return
			}
		}
	}
}

// Return the error that occurred during the import.
// NOTE: this method is not safe for concurrent use.
func (f *File) Err() error {
	return f.err
}

//============================================================================
// Exporter Methods
//============================================================================

// Export tasks to the zip file as long as the file is open for writing.
// NOTE: this method is not safe for concurrent use.
func (f *File) Task(tasks ...*task.Task) (err error) {
	if f.mode != ModeWrite && f.mode != ModeAppend {
		return fmt.Errorf("cannot export tasks to a file in %q mode", f.mode)
	}

	if f.writer == nil || f.manifest == nil {
		return ErrClosed
	}

	for _, task := range tasks {
		var entry *Entry
		if entry, err = f.manifest.Add(task); err != nil {
			return err
		}

		if err = f.writeFile(entry.Path, task); err != nil {
			return err
		}
	}

	return nil
}

//============================================================================
// Internal Functions
//============================================================================

func (f *File) loadManifest() (err error) {
	var z *zip.File
	if z, err = f.findManifest(); err != nil {
		return err
	}

	f.manifest = &Manifest{}
	return f.readFile(z, f.manifest)
}

func (f *File) findManifest() (z *zip.File, err error) {
	for _, z := range f.reader.File {
		if z.Name == "manifest.yaml" || z.Name == "manifest.json" {
			return z, nil
		}
	}
	return nil, ErrManifestNotFound
}

func (f *File) readFile(src, v any) (err error) {
	var (
		r    io.ReadCloser
		name string
	)

	switch src := src.(type) {
	case *zip.File:
		if r, err = src.Open(); err != nil {
			return err
		}
		name = src.Name
	case string:
		if r, err = f.reader.Open(src); err != nil {
			return err
		}
		name = src
	default:
		return fmt.Errorf("unsupported file type: %T", src)
	}
	defer r.Close()

	switch filepath.Ext(name) {
	case ".yaml", ".yml":
		if err = yaml.NewDecoder(r).Decode(v); err != nil {
			return err
		}
	case ".json":
		if err = json.NewDecoder(r).Decode(v); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported file extension: %q", filepath.Ext(name))
	}

	return nil
}

func (f *File) writeFile(name string, v any) (err error) {
	var w io.Writer
	if w, err = f.writer.Create(name); err != nil {
		return err
	}

	switch filepath.Ext(name) {
	case ".yaml", ".yml":
		return yaml.NewEncoder(w).Encode(v)
	case ".json":
		return json.NewEncoder(w).Encode(v)
	default:
		return fmt.Errorf("unsupported file extension: %q", filepath.Ext(name))
	}
}

//============================================================================
// Manifest Methods
//============================================================================

// Add a task to the manifest. Returns an error if the entry ID or slug has
// already been added to the manifest. A filename will be generated for the entry based
// on the slug or ID of the agent or task. If the entry has no slug or ID, then a ULID
// is created for the filename but will not be assigned to the entry or object.
func (m *Manifest) Add(obj any) (*Entry, error) {
	switch obj := obj.(type) {
	case *task.Task:
		return m.taskEntry(obj)
	default:
		return nil, fmt.Errorf("unsupported manifest object type: %T", obj)
	}
}

func (m *Manifest) taskEntry(task *task.Task) (entry *Entry, err error) {
	// Check to see if the task has already been added to the manifest.
	if !task.ID.IsZero() || task.Slug != "" {
		for _, other := range m.Tasks {
			if (!other.ID.IsZero() && other.ID == task.ID) || (other.Slug != "" && other.Slug == task.Slug) {
				return nil, ErrDuplicateTask
			}
		}
	}

	// Create the entry to add to the manifest.
	entry = &Entry{
		ID:   task.ID,
		Slug: task.Slug,
	}

	// Determine the filename for the task.
	switch {
	case entry.Slug != "":
		entry.Path = "tasks/" + normalize(entry.Slug) + DefaultExtension
	case !entry.ID.IsZero():
		entry.Path = "tasks/" + entry.ID.String() + DefaultExtension
	default:
		entry.Path = "tasks/" + ulid.Make().String() + DefaultExtension
	}

	// Determine the filename for the task.
	m.Tasks = append(m.Tasks, entry)
	return entry, nil
}

//============================================================================
// Helper Functions
//============================================================================

// Return an empty sequence of the given type.
func empty[T any]() iter.Seq[T] {
	return func(yield func(T) bool) {}
}

// Create a temporary archive file and copy the contents of the source into it. Returns
// the path to the temporary file, and a function to restore the original file if
// a subsequent operation fails. This is the first step in the append to archive process.
func tempArchive(in string) (out string, restore func() error, err error) {
	var tmp *os.File
	if tmp, err = os.CreateTemp("", "horizon-*.zip"); err != nil {
		return "", nil, fmt.Errorf("failed to create temporary file for horizon archive: %w", err)
	}
	defer tmp.Close()

	var f fs.File
	if f, err = os.Open(in); err != nil {
		return "", nil, fmt.Errorf("failed to open source file for horizon archive: %w", err)
	}
	defer f.Close()

	if _, err = io.Copy(tmp, f); err != nil {
		return "", nil, fmt.Errorf("failed to copy source file to temporary file: %w", err)
	}

	// The restore function is used to restore the original file if the operation fails.
	out = tmp.Name()
	restore = func() error {
		defer os.Remove(out)
		var dst *os.File
		if dst, err = os.Create(in); err != nil {
			return fmt.Errorf("could not restore original archive after error: %w", err)
		}
		defer dst.Close()

		var src *os.File
		if src, err = os.Open(out); err != nil {
			return fmt.Errorf("could not restore original archive after error: %w", err)
		}
		defer src.Close()

		if _, err = io.Copy(dst, src); err != nil {
			return fmt.Errorf("failed to copy restored archive to original file: %w", err)
		}

		return nil
	}

	return out, restore, nil
}

// Copy the contents of the source archive to the destination archive in preparation
// for making additions to the archive (used in the append path).
func copyArchive(dst *zip.Writer, src string) (err error) {
	// Open the source archive for reading.
	var s *zip.ReadCloser
	if s, err = zip.OpenReader(src); err != nil {
		return fmt.Errorf("failed to open temporary archive for copying: %w", err)
	}
	defer s.Close()

	// Copy the contents of the source archive to the destination archive.
	for _, f := range s.File {
		if err = dst.Copy(f); err != nil {
			return fmt.Errorf("failed to copy file %q from temporary archive to new appending archive: %w", f.Name, err)
		}
	}
	return nil
}

// Normalize a slug by converting it to lowercase and replacing dashes with underscores.
func normalize(slug string) string {
	slug = strings.ToLower(strings.ReplaceAll(slug, "-", "_"))
	slug = strings.Trim(slug, "/\\-_ \t\n\r\v\f")
	return slug
}
