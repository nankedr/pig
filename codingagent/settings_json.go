package codingagent

import (
	"bytes"
	"encoding/json"
	"fmt"
)

type settingsFields Settings

func (s Settings) fieldsJSON() (map[string]json.RawMessage, error) {
	data, err := json.Marshal(settingsFields(s))
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	if s.Compaction != nil {
		fields["compaction"], err = json.Marshal(map[string]any{"enabled": s.Compaction.Enabled, "reserveTokens": s.Compaction.ReserveTokens, "keepRecentTokens": s.Compaction.KeepRecentTokens})
	}
	return fields, err
}

func (s Settings) MarshalJSON() ([]byte, error) {
	fields, err := s.fieldsJSON()
	if err != nil {
		return nil, err
	}
	result := map[string]json.RawMessage{}
	for key, value := range s.original {
		result[key] = value
	}
	for key, value := range s.Extra {
		result[key] = value
	}
	for key := range s.decoded {
		if _, ok := fields[key]; !ok {
			delete(result, key)
		}
	}
	for key, value := range fields {
		if !bytes.Equal(value, s.decoded[key]) {
			result[key] = value
		}
	}
	return json.Marshal(result)
}

func (s *Settings) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw == nil {
		return fmt.Errorf("settings must be an object")
	}
	*s = Settings{}
	// Decode the PackageSource union separately; retain the original sparse JSON.
	copyFields := map[string]json.RawMessage{}
	for key, value := range raw {
		if key != "packages" {
			copyFields[key] = value
		}
	}
	encoded, err := json.Marshal(copyFields)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(encoded, (*settingsFields)(s)); err != nil {
		return err
	}
	if packages, ok := raw["packages"]; ok && string(packages) != "null" {
		var values []json.RawMessage
		if err = json.Unmarshal(packages, &values); err != nil {
			return err
		}
		s.Packages = make([]PackageSource, 0, len(values))
		for _, value := range values {
			var text string
			if json.Unmarshal(value, &text) == nil {
				s.Packages = append(s.Packages, StringPackageSource(text))
				continue
			}
			var source FilteredPackageSource
			if err = json.Unmarshal(value, &source); err != nil {
				return err
			}
			s.Packages = append(s.Packages, &source)
		}
	}
	s.original = raw
	s.decoded, err = s.fieldsJSON()
	if err != nil {
		return err
	}
	s.Extra = map[string]json.RawMessage{}
	for key, value := range raw {
		if _, ok := s.decoded[key]; !ok {
			s.Extra[key] = value
		}
	}
	return nil
}
