package config

import (
	"sync"
	"time"

	"go.rtnl.ai/confire"
)

// Prefix sets the prefix for Horizon environment variables.
const Prefix = "horizon"

// Config contains Horizon execution and rendering settings.
type Config struct {
	RendererCacheSize          int           `split_words:"true" default:"128" desc:"the size of the renderer cache"`
	ProviderCacheSize          int           `split_words:"true" default:"32" desc:"the maximum number of provider instances to cache"`
	MaxToolTurns               int64         `split_words:"true" default:"32" desc:"the maximum number of model/tool turns; 0 disables tool calling"`
	AttachmentMaxDownloadBytes int64         `split_words:"true" default:"67108864" desc:"the maximum number of bytes to download for an attachment in bytes (default 64mb)"`
	AttachmentDownloadTimeout  time.Duration `split_words:"true" default:"8s" desc:"the maximum duration of a remote attachment download"`
	ExecutionTimeout           time.Duration `split_words:"true" default:"0s" desc:"the maximum duration of one execution; 0 uses only the caller context"`
	FinalizeTimeout            time.Duration `split_words:"true" default:"8s" desc:"the maximum duration allowed for runner finalization"`
	ProviderRequestTimeout     time.Duration `split_words:"true" default:"128s" desc:"the maximum duration of an individual inference request"`
	HTTPClientTimeout          time.Duration `split_words:"true" default:"768s" desc:"the maximum duration of generic Horizon HTTP requests"`
	BestEffortParsing          bool          `split_words:"true" default:"false" desc:"set to true for best effort output parsing"`
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
	if c.ProviderCacheSize < 1 {
		err = confire.Join(err, confire.Invalid("horizon", "providerCacheSize", "must be greater than 0"))
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
	if c.ExecutionTimeout < 0 {
		err = confire.Join(err, confire.Invalid("horizon", "executionTimeout", "must not be negative"))
	}
	if c.FinalizeTimeout <= 0 {
		err = confire.Join(err, confire.Invalid("horizon", "finalizeTimeout", "must be greater than 0"))
	}
	if c.ProviderRequestTimeout <= 0 {
		err = confire.Join(err, confire.Invalid("horizon", "providerRequestTimeout", "must be greater than 0"))
	}
	if c.HTTPClientTimeout <= 0 {
		err = confire.Join(err, confire.Invalid("horizon", "httpClientTimeout", "must be greater than 0"))
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
