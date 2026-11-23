package doc

import (
	"fmt"
	"os"
	"strings"
	"unicode"
)

// BibEntry holds a single parsed BibTeX entry.
type BibEntry struct {
	Key       string
	Type      string
	Title     string
	Author    string
	Authors   []string
	Year      string
	Journal   string
	Booktitle string
	Publisher string
	Volume    string
	Number    string
	Pages     string
	DOI       string
	URL       string
	Note      string
	Fields    map[string]string
}

// ParseBibFile reads and parses a BibTeX (.bib) file from disk.
func ParseBibFile(path string) (map[string]BibEntry, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseBib(raw)
}

// ParseBib parses raw BibTeX bytes into a map keyed by citation key.
func ParseBib(src []byte) (map[string]BibEntry, error) {
	p := &bibParser{
		src:     string(src),
		strings: make(map[string]string),
	}
	return p.parse()
}

type bibParser struct {
	src     string
	pos     int
	strings map[string]string
}

func (p *bibParser) parse() (map[string]BibEntry, error) {
	entries := make(map[string]BibEntry)
	for p.pos < len(p.src) {
		p.skipSpaceAndComments()
		if p.pos >= len(p.src) {
			break
		}
		if p.src[p.pos] != '@' {
			p.pos++
			continue
		}
		p.pos++ // skip '@'
		entryType := strings.ToLower(strings.TrimSpace(p.readIdent()))
		p.skipSpaceAndComments()
		if p.pos >= len(p.src) {
			break
		}
		openDelim := p.src[p.pos]
		var closeDelim byte
		switch openDelim {
		case '{':
			closeDelim = '}'
		case '(':
			closeDelim = ')'
		default:
			continue
		}
		p.pos++ // skip opening delim

		switch entryType {
		case "comment":
			p.skipBalanced(openDelim, closeDelim)
			continue
		case "preamble":
			p.skipBalanced(openDelim, closeDelim)
			continue
		case "string":
			p.parseStringDef(closeDelim)
			continue
		}

		p.skipSpaceAndComments()
		key := strings.TrimSpace(p.readUntilAny(",\r\n" + string(closeDelim)))
		p.skipSpaceAndComments()
		if p.pos < len(p.src) && p.src[p.pos] == ',' {
			p.pos++
		}

		fields := p.parseFields(closeDelim)
		if key == "" {
			continue
		}
		entries[key] = buildBibEntry(key, entryType, fields)
	}
	return entries, nil
}

func (p *bibParser) parseStringDef(closeDelim byte) {
	p.skipSpaceAndComments()
	name := strings.ToLower(strings.TrimSpace(p.readIdent()))
	p.skipSpaceAndComments()
	if p.pos < len(p.src) && p.src[p.pos] == '=' {
		p.pos++
		val := p.parseValue()
		if name != "" {
			p.strings[name] = val
		}
	}
	for p.pos < len(p.src) && p.src[p.pos] != closeDelim {
		p.pos++
	}
	if p.pos < len(p.src) && p.src[p.pos] == closeDelim {
		p.pos++
	}
}

func (p *bibParser) parseFields(closeDelim byte) map[string]string {
	fields := make(map[string]string)
	for p.pos < len(p.src) {
		p.skipSpaceAndComments()
		if p.pos >= len(p.src) {
			break
		}
		if p.src[p.pos] == closeDelim {
			p.pos++
			break
		}
		if p.src[p.pos] == ',' {
			p.pos++
			continue
		}
		name := strings.ToLower(strings.TrimSpace(p.readIdent()))
		p.skipSpaceAndComments()
		if name == "" {
			// Avoid infinite loop on malformed tokens.
			p.pos++
			continue
		}
		if p.pos >= len(p.src) || p.src[p.pos] != '=' {
			for p.pos < len(p.src) && p.src[p.pos] != ',' && p.src[p.pos] != closeDelim {
				p.pos++
			}
			continue
		}
		p.pos++ // skip '='
		val := p.parseValue()
		fields[name] = val
	}
	return fields
}

func (p *bibParser) parseValue() string {
	var b strings.Builder
	for {
		p.skipSpaceAndComments()
		if p.pos >= len(p.src) {
			break
		}
		c := p.src[p.pos]
		switch c {
		case '{':
			b.WriteString(p.readBraced())
		case '"':
			b.WriteString(p.readQuoted())
		default:
			tok := p.readBareToken()
			if tok != "" {
				if sub, ok := p.strings[strings.ToLower(tok)]; ok {
					b.WriteString(sub)
				} else {
					b.WriteString(tok)
				}
			}
		}
		p.skipSpaceAndComments()
		if p.pos < len(p.src) && p.src[p.pos] == '#' {
			p.pos++
			continue
		}
		break
	}
	return strings.TrimSpace(b.String())
}

func (p *bibParser) readBraced() string {
	if p.pos >= len(p.src) || p.src[p.pos] != '{' {
		return ""
	}
	p.pos++
	start := p.pos
	depth := 1
	for p.pos < len(p.src) && depth > 0 {
		c := p.src[p.pos]
		if c == '\\' && p.pos+1 < len(p.src) {
			p.pos += 2
			continue
		}
		if c == '{' {
			depth++
		} else if c == '}' {
			depth--
			if depth == 0 {
				val := p.src[start:p.pos]
				p.pos++
				return val
			}
		}
		p.pos++
	}
	return p.src[start:p.pos]
}

func (p *bibParser) readQuoted() string {
	if p.pos >= len(p.src) || p.src[p.pos] != '"' {
		return ""
	}
	p.pos++
	start := p.pos
	depth := 0
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		if c == '\\' && p.pos+1 < len(p.src) {
			p.pos += 2
			continue
		}
		if c == '{' {
			depth++
		} else if c == '}' && depth > 0 {
			depth--
		} else if c == '"' && depth == 0 {
			val := p.src[start:p.pos]
			p.pos++
			return val
		}
		p.pos++
	}
	return p.src[start:p.pos]
}

func (p *bibParser) readBareToken() string {
	start := p.pos
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		if c == ',' || c == '}' || c == ')' || c == '#' || c == '=' || isBibSpace(c) {
			break
		}
		p.pos++
	}
	return p.src[start:p.pos]
}

func (p *bibParser) readIdent() string {
	start := p.pos
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		if isBibSpace(c) || c == '=' || c == ',' || c == '{' || c == '(' || c == '}' || c == ')' {
			break
		}
		p.pos++
	}
	return p.src[start:p.pos]
}

func (p *bibParser) readUntilAny(stop string) string {
	start := p.pos
	for p.pos < len(p.src) {
		if strings.IndexByte(stop, p.src[p.pos]) >= 0 {
			break
		}
		p.pos++
	}
	return p.src[start:p.pos]
}

func (p *bibParser) skipBalanced(openDelim, closeDelim byte) {
	depth := 1
	for p.pos < len(p.src) && depth > 0 {
		c := p.src[p.pos]
		if c == '\\' && p.pos+1 < len(p.src) {
			p.pos += 2
			continue
		}
		if c == openDelim {
			depth++
		} else if c == closeDelim {
			depth--
		}
		p.pos++
	}
}

func (p *bibParser) skipSpaceAndComments() {
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		if isBibSpace(c) {
			p.pos++
			continue
		}
		if c == '%' {
			for p.pos < len(p.src) && p.src[p.pos] != '\n' {
				p.pos++
			}
			continue
		}
		break
	}
}

func isBibSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

func buildBibEntry(key, entryType string, rawFields map[string]string) BibEntry {
	fields := make(map[string]string, len(rawFields))
	for k, v := range rawFields {
		fields[k] = cleanBibText(v)
	}
	rawAuthor := rawFields["author"]
	authors := splitBibAuthors(rawAuthor)
	for i := range authors {
		authors[i] = cleanBibText(authors[i])
	}

	return BibEntry{
		Key:       key,
		Type:      entryType,
		Title:     fields["title"],
		Author:    strings.Join(authors, ", "),
		Authors:   authors,
		Year:      fields["year"],
		Journal:   fields["journal"],
		Booktitle: fields["booktitle"],
		Publisher: fields["publisher"],
		Volume:    fields["volume"],
		Number:    fields["number"],
		Pages:     cleanBibPages(fields["pages"]),
		DOI:       fields["doi"],
		URL:       fields["url"],
		Note:      fields["note"],
		Fields:    fields,
	}
}

func splitBibAuthors(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var out []string
	depth := 0
	start := 0
	lower := strings.ToLower(raw)
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if c == '{' {
			depth++
		} else if c == '}' && depth > 0 {
			depth--
		} else if depth == 0 && i+5 <= len(raw) && lower[i:i+5] == " and " {
			part := strings.TrimSpace(raw[start:i])
			if part != "" {
				out = append(out, part)
			}
			start = i + 5
			i += 4
		}
	}
	if tail := strings.TrimSpace(raw[start:]); tail != "" {
		out = append(out, tail)
	}
	return out
}

func cleanBibPages(s string) string {
	s = strings.ReplaceAll(s, "---", "—")
	s = strings.ReplaceAll(s, "--", "–")
	return s
}

func cleanBibText(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == '\\' && i+1 < len(s) {
			if rep, consumed, ok := decodeLatexAccent(s[i:]); ok {
				b.WriteString(rep)
				i += consumed
				continue
			}
			next := s[i+1]
			switch next {
			case '&', '%', '$', '#', '_', '{', '}':
				b.WriteByte(next)
				i += 2
				continue
			}
		}
		if s[i] == '{' || s[i] == '}' {
			i++
			continue
		}
		if s[i] == '\n' || s[i] == '\r' || s[i] == '\t' {
			b.WriteByte(' ')
			i++
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return collapseSpaces(b.String())
}

func decodeLatexAccent(s string) (string, int, bool) {
	if len(s) < 3 || s[0] != '\\' {
		return "", 0, false
	}
	cmd := s[1]
	switch cmd {
	case '\'', '`', '^', '"', '~', '=', '.', 'c', 'v', 'u', 'H', 'r', 'k':
	default:
		return "", 0, false
	}
	idx := 2
	for idx < len(s) && s[idx] == ' ' {
		idx++
	}
	if idx >= len(s) {
		return "", 0, false
	}
	var base byte
	consumed := idx + 1
	if s[idx] == '{' {
		closeIdx := strings.IndexByte(s[idx+1:], '}')
		if closeIdx < 0 {
			return "", 0, false
		}
		inner := strings.TrimSpace(s[idx+1 : idx+1+closeIdx])
		if len(inner) == 0 {
			return "", 0, false
		}
		if inner == `\i` {
			base = 'i'
		} else {
			base = inner[0]
		}
		consumed = idx + 1 + closeIdx + 1
	} else {
		base = s[idx]
	}

	key := fmt.Sprintf("%c%c", cmd, base)
	if rep, ok := latexAccentMap[key]; ok {
		return rep, consumed, true
	}
	return string(base), consumed, true
}

func collapseSpaces(s string) string {
	var b strings.Builder
	inSpace := false
	for _, r := range strings.TrimSpace(s) {
		if unicode.IsSpace(r) {
			if !inSpace {
				b.WriteByte(' ')
				inSpace = true
			}
			continue
		}
		inSpace = false
		b.WriteRune(r)
	}
	return b.String()
}

// AuthorSurname returns the family name for an author formatted as "Last, First" or "First Last".
func AuthorSurname(author string) string {
	author = strings.TrimSpace(author)
	if author == "" {
		return ""
	}
	if comma := strings.IndexByte(author, ','); comma >= 0 {
		return strings.TrimSpace(author[:comma])
	}
	fields := strings.Fields(author)
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}

var latexAccentMap = map[string]string{
	`'a`: "á", `'e`: "é", `'i`: "í", `'o`: "ó", `'u`: "ú", `'y`: "ý",
	`'A`: "Á", `'E`: "É", `'I`: "Í", `'O`: "Ó", `'U`: "Ú", `'Y`: "Ý",
	`'c`: "ć", `'C`: "Ć", `'n`: "ń", `'N`: "Ń", `'s`: "ś", `'S`: "Ś",
	`'z`: "ź", `'Z`: "Ź",
	"`a": "à", "`e": "è", "`i": "ì", "`o": "ò", "`u": "ù",
	"`A": "À", "`E": "È", "`I": "Ì", "`O": "Ò", "`U": "Ù",
	`^a`: "â", `^e`: "ê", `^i`: "î", `^o`: "ô", `^u`: "û",
	`^A`: "Â", `^E`: "Ê", `^I`: "Î", `^O`: "Ô", `^U`: "Û",
	`"a`: "ä", `"e`: "ë", `"i`: "ï", `"o`: "ö", `"u`: "ü", `"y`: "ÿ",
	`"A`: "Ä", `"E`: "Ë", `"I`: "Ï", `"O`: "Ö", `"U`: "Ü",
	`~a`: "ã", `~o`: "õ", `~n`: "ñ", `~A`: "Ã", `~O`: "Õ", `~N`: "Ñ",
	`cc`: "ç", `cC`: "Ç",
	`vc`: "č", `vC`: "Č", `vs`: "š", `vS`: "Š", `vz`: "ž", `vZ`: "Ž",
	`ra`: "å", `rA`: "Å",
}
