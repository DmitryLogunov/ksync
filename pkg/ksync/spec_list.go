package ksync

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"reflect"

	homedir "github.com/mitchellh/go-homedir"
	"github.com/mitchellh/mapstructure"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cast"
	"github.com/spf13/viper"
	yaml "gopkg.in/yaml.v2"

	"github.com/vapor-ware/ksync/pkg/debug"
)

// SpecList is a list of specs.
type SpecList struct {
	Items map[string]*Spec
}

func (s *SpecList) String() string {
	return debug.YamlString(s)
}

// Fields returns a set of structured fields for logging.
func (s *SpecList) Fields() log.Fields {
	return log.Fields{}
}

func allSpecs() (map[string]*Spec, error) {
	items := map[string]*Spec{}

	for _, raw := range cast.ToSlice(viper.Get("spec")) {
		var spec Spec
		if err := mapstructure.Decode(raw, &spec); err != nil {
			return nil, err
		}

		// TODO: validate the spec
		items[spec.Name] = &spec
	}

	return items, nil
}

// Update looks at config and updates the SpecList to the latest state on disk,
// cleaning any items that are removed.
func (s *SpecList) Update() error {
	if s.Items == nil {
		s.Items = map[string]*Spec{}
	}

	newItems, err := allSpecs()
	if err != nil {
		return err
	}

	// there are new specs to monitor
	for name, spec := range newItems {
		if _, ok := s.Items[name]; !ok {
			s.Items[name] = spec
		}
	}

	// there have been specs removed
	for name, spec := range s.Items {
		if _, ok := newItems[name]; !ok {
			if err := spec.Cleanup(); err != nil {
				return err
			}
			delete(s.Items, name)
		}
	}

	return nil
}

// Watch will start watching every item in the list.
func (s *SpecList) Watch() error {
	for _, spec := range s.Items {
		if err := spec.Watch(); err != nil {
			return err
		}
	}

	return nil
}

// Create checks an individual input spec for likeness and duplicates
// then adds the spec into a SpecList
func (s *SpecList) Create(name string, spec *Spec, force bool) error {
	if !force {
		if s.Has(name) {
			// TODO: make this into a type?
			return fmt.Errorf("name already exists")
		}

		if s.HasLike(spec) {
			return fmt.Errorf("similar spec exists")
		}
	}

	s.Items[name] = spec
	return nil
}

// Delete removes a given spec from a SpecList
func (s *SpecList) Delete(name string) error {
	if !s.Has(name) {
		return fmt.Errorf("does not exist")
	}

	delete(s.Items, name)
	return nil
}

// Save serializes the current SpecList's items to the config file.
// TODO: tests:
//   missing config file
//   shorter config file (removing an entry)
func (s *SpecList) Save() error {
	cfgPath := viper.ConfigFileUsed()
	if cfgPath == "" {
		home, err := homedir.Dir()
		if err != nil {
			return err
		}

		cfgPath = filepath.Join(home, fmt.Sprintf(".%s.yaml", "ksync"))
	}

	log.WithFields(log.Fields{
		"path": cfgPath,
	}).Debug("writing config file")

	var specs []*Spec
	for _, v := range s.Items {
		specs = append(specs, v)
	}
	viper.Set("spec", specs)
	buf, err := yaml.Marshal(viper.AllSettings())
	if err != nil {
		return err
	}

	return ioutil.WriteFile(cfgPath, buf, 0644)
}

// HasLike checks a given spec for deep equivalence against another spec
// TODO: is this the best way to do this?
func (s *SpecList) HasLike(target *Spec) bool {
	targetEq := target.Equivalence()
	for _, spec := range s.Items {
		if reflect.DeepEqual(targetEq, spec.Equivalence()) {
			return true
		}
	}
	return false
}

// Has checks a given spec for simple equivalence against another spec
func (s *SpecList) Has(target string) bool {
	if _, ok := s.Items[target]; ok {
		return true
	}
	return false
}
