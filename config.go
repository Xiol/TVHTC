package main

import (
	"fmt"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"regexp"
	"sync"
)

type Config struct {
	sync.RWMutex
	FromAddress   string             `yaml:"from_addr"`
	EmailHost     string             `yaml:"email_host"`
	PushoverToken string             `yaml:"pushover_app_token"`
	KeepOriginals bool               `yaml:"keep_originals"`
	TCSettings    TranscodeSettings  `yaml:"transcode_settings"`
	NotifyList    map[string]*Person `yaml:"notify_list"`
	TrimPath      string             `yaml:"trim_path"`
}

type TranscodeSettings struct {
	Audio string `yaml:"audio"`
	Video string `yaml:"video"`
}

type Person struct {
	sync.RWMutex
	Name         string           `yaml:"-"`
	Email        string           `yaml:"email"`
	Pushover     string           `yaml:"pushover"`
	NotifyFor    []string         `yaml:"notify_for"`
	InterestedIn []*regexp.Regexp `yaml:"-"`
	IsDefault    bool             `yaml:"is_default"`
}

func NewConfig() Config {
	return Config{}
}

func (this *Config) Load(path string) error {
	this.Lock()
	defer this.Unlock()

	raw, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}

	err = yaml.Unmarshal(raw, this)
	if err != nil {
		return err
	}

	for name, _ := range this.NotifyList {
		this.NotifyList[name].Lock()
		defer this.NotifyList[name].Unlock()

		for name := range this.NotifyList {
			this.NotifyList[name].Name = name
		}

		this.NotifyList[name].InterestedIn = make([]*regexp.Regexp, len(this.NotifyList[name].NotifyFor))
		for i := range this.NotifyList[name].NotifyFor {
			r, err := regexp.Compile(fmt.Sprintf("(?i)%v", this.NotifyList[name].NotifyFor[i]))
			if err != nil {
				return fmt.Errorf("Regexp compilation failure: %v", err)
			}
			this.NotifyList[name].InterestedIn[i] = r
		}
	}

	return nil
}

func (this *Person) NotificationWanted(title string) bool {
	this.RLock()
	defer this.RUnlock()

	for i := range this.InterestedIn {
		if this.InterestedIn[i].Match([]byte(title)) {
			Log.Debug("Person %v wants notification for '%v'", this.Name, title)
			return true
		}
	}
	Log.Debug("Person %v does not want notification for '%v'", this.Name, title)
	return false
}
