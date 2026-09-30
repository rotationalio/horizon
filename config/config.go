package config

import (
	"sync"
	"time"

	"go.rtnl.ai/confire"
)

// Prefix sets the prefix for Horizon environment variables.
const Prefix = "horizon"

// DefaultMaxToolTurns bounds the number of model/tool exchanges in one run.
const DefaultMaxToolTurns = 32

const (
	// DefaultAttachmentMaxDownloadBytes limits remote attachment downloads to 64 MiB.
	DefaultAttachmentMaxDownloadBytes int64 = 64 << 20
	// DefaultAttachmentDownloadTimeout limits remote attachment downloads to 16 seconds.
	DefaultAttachmentDownloadTimeout = 16 * time.Second
)

// Config contains Horizon execution and rendering settings.
type Config struct {
	RendererCacheSize          int           `split_words:"true" default:"128" desc:"the size of the renderer cache"`
	MaxToolTurns               int64         `split_words:"true" default:"32" desc:"the maximum number of model/tool turns; 0 disables tool calling"`
	AttachmentMaxDownloadBytes int64         `split_words:"true" default:"67108864" desc:"the maximum number of bytes to download for an attachment (default 64mb)"`
	AttachmentDownloadTimeout  time.Duration `split_words:"true" default:"16s" desc:"the maximum duration of a remote attachment download"`
}

func New() (conf *Config, err error) {
	conf = &Config{}
	if err = confire.Process(Prefix, conf); err != nil {
		return nil, err
	}
	return conf, nil
}

func (c Config) Validate() (err error) {
	if c.RendererCacheSize < 1 {
		err = confire.Join(err, confire.Invalid("horizon", "rendererCacheSize", "must be greater than 0"))
	}
	if c.MaxToolTurns < 0 {
		err = confire.Join(err, confire.Invalid("horizon", "maxToolTurns", "must not be negative"))
	}
	if c.AttachmentMaxDownloadBytes <= 0 {
		err = confire.Join(err, confire.Invalid("horizon", "attachmentMaxDownloadBytes", "must be greater than 0"))
	}
	if c.AttachmentDownloadTimeout <= 0 {
		err = confire.Join(err, confire.Invalid("horizon", "attachmentDownloadTimeout", "must be greater than 0"))
	}
	return err
}

//============================================================================
// Config Package Management
//============================================================================

var (
	mu   sync.RWMutex
	err  error // only written by New() inside Get's sync.Once, and cleared on successful Set
	load sync.Once
	conf *Config
)

func Get() (Config, error) {
	load.Do(func() {
		mu.Lock()
		defer mu.Unlock()

		if conf == nil {
			conf, err = New()
		}
	})
	mu.RLock()
	defer mu.RUnlock()
	if conf != nil {
		return *conf, err
	}
	return Config{}, err
}

func Set(c Config) error {
	if err := c.Validate(); err != nil {
		return err
	}

	mu.Lock()
	defer mu.Unlock()

	conf = &c
	err = nil
	return nil
}

func Reset() {
	mu.Lock()
	defer mu.Unlock()

	conf = nil
	err = nil
	load = sync.Once{}
}
