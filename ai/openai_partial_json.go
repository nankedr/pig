package ai

import (
	"encoding/json"
	"errors"
	"strings"
)

func (value *openAIStreamingString) UnmarshalJSON(data []byte) error {
	decoded, ok := decodeOpenAIJSONString(string(data), true)
	if !ok {
		return errors.New("invalid streaming arguments string")
	}
	value.value = decoded
	return nil
}

func encodeOpenAIArgumentFragment(fragment string) string {
	var encoded strings.Builder
	encoded.Grow(len(fragment))
	for i := 0; i < len(fragment); i++ {
		if i+2 < len(fragment) && fragment[i] == 0xed && fragment[i+1]&0xe0 == 0xa0 && fragment[i+2]&0xc0 == 0x80 {
			unit := uint16(fragment[i]&0x0f)<<12 | uint16(fragment[i+1]&0x3f)<<6 | uint16(fragment[i+2]&0x3f)
			if unit >= 0xd800 && unit <= 0xdfff {
				writeOpenAIUnicodeEscape(&encoded, unit)
				i += 2
				continue
			}
		}
		encoded.WriteByte(fragment[i])
	}
	return encoded.String()
}

// ParseStreamingJSONObject reconstructs object arguments from incomplete JSON.
// Missing or malformed fragments yield the recoverable object, or an empty map.
func ParseStreamingJSONObject(raw string) map[string]any { return parseOpenAIPartialObject(raw) }

func parseOpenAIPartialObject(raw string) map[string]any {
	var object map[string]any
	if json.Unmarshal([]byte(raw), &object) == nil && object != nil {
		return object
	}
	repaired := repairOpenAIJSON(raw)
	if repaired != raw && json.Unmarshal([]byte(repaired), &object) == nil && object != nil {
		return object
	}
	parser := openAIPartialJSONParser{input: strings.TrimSpace(raw)}
	value, ok := parser.parseValue()
	if !ok && repaired != raw {
		parser = openAIPartialJSONParser{input: strings.TrimSpace(repaired)}
		value, ok = parser.parseValue()
	}
	if !ok {
		return map[string]any{}
	}
	object, _ = value.(map[string]any)
	if object == nil {
		return map[string]any{}
	}
	return object
}

func repairOpenAIJSON(raw string) string {
	var repaired strings.Builder
	repaired.Grow(len(raw))
	inString := false
	for i := 0; i < len(raw); i++ {
		char := raw[i]
		if !inString {
			repaired.WriteByte(char)
			if char == '"' {
				inString = true
			}
			continue
		}
		if char == '"' {
			repaired.WriteByte(char)
			inString = false
			continue
		}
		if char == '\\' {
			if i+1 < len(raw) && validOpenAIJSONEscape(raw[i+1]) {
				if raw[i+1] != 'u' || i+5 < len(raw) && validOpenAIHex(raw[i+2:i+6]) {
					repaired.WriteString(raw[i : i+2])
					i++
					if raw[i] == 'u' {
						repaired.WriteString(raw[i+1 : i+5])
						i += 4
					}
					continue
				}
			}
			repaired.WriteString(`\\`)
			continue
		}
		if char <= 0x1f {
			switch char {
			case '\b':
				repaired.WriteString(`\b`)
			case '\f':
				repaired.WriteString(`\f`)
			case '\n':
				repaired.WriteString(`\n`)
			case '\r':
				repaired.WriteString(`\r`)
			case '\t':
				repaired.WriteString(`\t`)
			default:
				hex := "0123456789abcdef"
				repaired.WriteString(`\u00`)
				repaired.WriteByte(hex[char>>4])
				repaired.WriteByte(hex[char&0xf])
			}
			continue
		}
		repaired.WriteByte(char)
	}
	return repaired.String()
}

func validOpenAIJSONEscape(char byte) bool {
	return strings.ContainsRune(`"\\/bfnrtu`, rune(char))
}

func validOpenAIHex(value string) bool {
	if len(value) != 4 {
		return false
	}
	for i := range value {
		char := value[i]
		if !(char >= '0' && char <= '9' || char >= 'a' && char <= 'f' || char >= 'A' && char <= 'F') {
			return false
		}
	}
	return true
}

type openAIPartialJSONParser struct {
	input string
	index int
}

func (p *openAIPartialJSONParser) parseValue() (any, bool) {
	p.skipBlank()
	if p.index >= len(p.input) {
		return nil, false
	}
	switch p.input[p.index] {
	case '"':
		return p.parseString()
	case '{':
		return p.parseObject()
	case '[':
		return p.parseArray()
	}
	remaining := p.input[p.index:]
	for _, literal := range []struct {
		text  string
		value any
	}{{"null", nil}, {"true", true}, {"false", false}} {
		if strings.HasPrefix(remaining, literal.text) || len(remaining) < len(literal.text) && strings.HasPrefix(literal.text, remaining) {
			p.index += len(literal.text)
			return literal.value, true
		}
	}
	return p.parseNumber()
}

func (p *openAIPartialJSONParser) parseString() (any, bool) {
	start := p.index
	p.index++
	escaped := false
	for p.index < len(p.input) {
		char := p.input[p.index]
		if char == '"' && !escaped {
			p.index++
			value, ok := decodeOpenAIJSONString(p.input[start:p.index], true)
			return value, ok
		}
		if char == '\\' {
			escaped = !escaped
		} else {
			escaped = false
		}
		p.index++
	}
	end := p.index
	if escaped {
		end--
	}
	if value, ok := decodeOpenAIJSONString(p.input[start:end]+`"`, true); ok {
		return value, true
	}
	if slash := strings.LastIndex(p.input[start:p.index], `\`); slash >= 0 {
		value, ok := decodeOpenAIJSONString(p.input[start:start+slash]+`"`, true)
		return value, ok
	}
	return nil, false
}

func decodeOpenAIJSONString(candidate string, preserveSurrogates bool) (string, bool) {
	if len(candidate) < 2 || candidate[0] != '"' || candidate[len(candidate)-1] != '"' {
		return "", false
	}
	var decoded strings.Builder
	decoded.Grow(len(candidate) - 2)
	end := len(candidate) - 1
	for i := 1; i < end; i++ {
		char := candidate[i]
		if char != '\\' {
			if char <= 0x1f || char == '"' {
				return "", false
			}
			decoded.WriteByte(char)
			continue
		}
		if i+1 >= end {
			return "", false
		}
		i++
		switch candidate[i] {
		case '"', '\\', '/':
			decoded.WriteByte(candidate[i])
		case 'b':
			decoded.WriteByte('\b')
		case 'f':
			decoded.WriteByte('\f')
		case 'n':
			decoded.WriteByte('\n')
		case 'r':
			decoded.WriteByte('\r')
		case 't':
			decoded.WriteByte('\t')
		case 'u':
			if i+4 >= end {
				return "", false
			}
			unit, ok := parseOpenAIHex4(candidate[i+1 : i+5])
			if !ok {
				return "", false
			}
			i += 4
			if unit >= 0xd800 && unit <= 0xdbff && i+6 < end && candidate[i+1:i+3] == `\u` {
				low, valid := parseOpenAIHex4(candidate[i+3 : i+7])
				if valid && low >= 0xdc00 && low <= 0xdfff {
					decoded.WriteRune(rune(0x10000 + (uint32(unit)-0xd800)<<10 + uint32(low) - 0xdc00))
					i += 6
					continue
				}
			}
			writeOpenAIJSONCodeUnit(&decoded, unit, preserveSurrogates)
		default:
			return "", false
		}
	}
	return decoded.String(), true
}

func writeOpenAIJSONCodeUnit(target *strings.Builder, unit uint16, preserveSurrogate bool) {
	if unit < 0xd800 || unit > 0xdfff {
		target.WriteRune(rune(unit))
		return
	}
	if preserveSurrogate {
		target.WriteByte(0xe0 | byte(unit>>12))
		target.WriteByte(0x80 | byte(unit>>6)&0x3f)
		target.WriteByte(0x80 | byte(unit)&0x3f)
		return
	}
	target.WriteString("\xef\xbf\xbd")
}

func writeOpenAIUnicodeEscape(target *strings.Builder, unit uint16) {
	hex := "0123456789abcdef"
	target.WriteString(`\u`)
	target.WriteByte(hex[unit>>12])
	target.WriteByte(hex[unit>>8&0xf])
	target.WriteByte(hex[unit>>4&0xf])
	target.WriteByte(hex[unit&0xf])
}

func parseOpenAIHex4(value string) (uint16, bool) {
	if len(value) != 4 {
		return 0, false
	}
	var result uint16
	for i := range value {
		result <<= 4
		switch char := value[i]; {
		case char >= '0' && char <= '9':
			result |= uint16(char - '0')
		case char >= 'a' && char <= 'f':
			result |= uint16(char-'a') + 10
		case char >= 'A' && char <= 'F':
			result |= uint16(char-'A') + 10
		default:
			return 0, false
		}
	}
	return result, true
}

func (p *openAIPartialJSONParser) parseObject() (any, bool) {
	p.index++
	p.skipBlank()
	object := map[string]any{}
	for {
		p.skipBlank()
		if p.index >= len(p.input) {
			return object, true
		}
		if p.input[p.index] == '}' {
			p.index++
			return object, true
		}
		key, ok := p.parseString()
		if !ok {
			return object, true
		}
		p.skipBlank()
		p.index++
		value, ok := p.parseValue()
		if !ok {
			return object, true
		}
		object[key.(string)] = value
		p.skipBlank()
		if p.index < len(p.input) && p.input[p.index] == ',' {
			p.index++
		}
	}
}

func (p *openAIPartialJSONParser) parseArray() (any, bool) {
	p.index++
	array := []any{}
	for {
		p.skipBlank()
		if p.index < len(p.input) && p.input[p.index] == ']' {
			p.index++
			return array, true
		}
		value, ok := p.parseValue()
		if !ok {
			return array, true
		}
		array = append(array, value)
		p.skipBlank()
		if p.index < len(p.input) && p.input[p.index] == ',' {
			p.index++
		}
	}
}

func (p *openAIPartialJSONParser) parseNumber() (any, bool) {
	start := p.index
	if p.input[p.index] == '-' {
		p.index++
	}
	for p.index < len(p.input) && !strings.ContainsRune(",]}", rune(p.input[p.index])) {
		p.index++
	}
	token := p.input[start:p.index]
	if token == "-" {
		return nil, false
	}
	var value any
	if json.Unmarshal([]byte(token), &value) == nil {
		return value, true
	}
	if exponent := strings.LastIndexByte(token, 'e'); exponent >= 0 && json.Unmarshal([]byte(token[:exponent]), &value) == nil {
		return value, true
	}
	return nil, false
}

func (p *openAIPartialJSONParser) skipBlank() {
	for p.index < len(p.input) && strings.ContainsRune(" \n\r\t", rune(p.input[p.index])) {
		p.index++
	}
}
